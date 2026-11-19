// Package llm generates jq filters with a language model run by a command line tool.
package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"kfee/config"
	"kfee/serdes"
)

// Timeout bounds a single generation.
const Timeout = 3 * time.Minute

// MaxAttachmentBytes bounds the attached files together.
const MaxAttachmentBytes = 256 * 1024

// MaxArgumentBytes bounds a prompt passed as a command line argument, Linux rejects longer ones.
const MaxArgumentBytes = 120 * 1024

// systemPrompt is sent with every request. It asks for the filter only, so the output can be
// used as is.
const systemPrompt = `You write jq filters. The input of the filter is one JSON document, a protobuf message
converted to JSON with the field names of its schema. The schema is given below. Enum values are
shown with their names. Attached files may contain mappings, use them to replace codes with readable values.

Reply with the jq filter only: no explanation, no markdown and no code fences.
If the request cannot be expressed as a jq filter, reply with one line starting with "ERROR:" followed by the reason.`

// Request describes what the filter should do.
type Request struct {
	// Description is what the user wants, in their own words.
	Description string
	// Schema is the message type with the types it uses, see serdes.ProtoSchema.Describe.
	Schema string
	// Sample is a decoded record, only included when the configuration allows it.
	Sample string
	// Attachments are files with additional information, e.g. mappings of codes to names.
	Attachments []Attachment
}

// Attachment is a file included in the prompt.
type Attachment struct {
	Path    string
	Content string
}

// ReadAttachments reads the files, their paths may start with ~.
func ReadAttachments(paths []string) ([]Attachment, error) {
	var attachments []Attachment
	total := 0
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		data, err := os.ReadFile(serdes.ExpandHome(p))
		if err != nil {
			return nil, fmt.Errorf("unable to read attachment: %w", err)
		}
		total += len(data)
		if total > MaxAttachmentBytes {
			return nil, fmt.Errorf("attachments exceed %d KiB", MaxAttachmentBytes/1024)
		}
		attachments = append(attachments, Attachment{Path: p, Content: string(data)})
	}
	return attachments, nil
}

// BuildPrompt returns the prompt for the request: the description, the schema and the optional
// sample and attachments. The system prompt is included in the returned text.
func BuildPrompt(r Request) string {
	var b strings.Builder
	b.WriteString(systemPrompt)
	b.WriteString("\n\n## Schema\n")
	b.WriteString(r.Schema)
	if r.Sample != "" {
		b.WriteString("\n## Sample input\n")
		b.WriteString(r.Sample)
		b.WriteString("\n")
	}
	for _, a := range r.Attachments {
		fmt.Fprintf(&b, "\n## Attached file %s\n%s\n", a.Path, a.Content)
	}
	b.WriteString("\n## Request\n")
	b.WriteString(r.Description)
	b.WriteString("\n")
	return b.String()
}

// Run runs the configured command with the prompt and returns its standard output.
func Run(ctx context.Context, cfg config.LLMConfig, prompt string) (string, error) {
	if !cfg.Configured() {
		return "", errors.New("no LLM command configured")
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	if cfg.PromptAsArgument && len(prompt) > MaxArgumentBytes {
		return "", fmt.Errorf("the prompt is %d KiB, too large to pass as an argument: use smaller attachments or a tool reading stdin", len(prompt)/1024)
	}
	args := append([]string{}, cfg.Command[1:]...)
	if cfg.PromptAsArgument {
		args = append(args, prompt)
	}
	cmd := exec.CommandContext(ctx, cfg.Command[0], args...)
	if !cfg.PromptAsArgument {
		cmd.Stdin = strings.NewReader(prompt)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s timed out after %s", cfg.Command[0], Timeout)
		}
		return "", fmt.Errorf("%s failed: %w: %s", cfg.Command[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// ParseFilter extracts the filter from the output of the model. Code fences are removed,
// an ERROR reply becomes an error.
func ParseFilter(output string) (string, error) {
	filter := strings.TrimSpace(output)
	if strings.HasPrefix(filter, "```") {
		lines := strings.Split(filter, "\n")
		lines = lines[1:]
		if n := len(lines); n > 0 && strings.HasPrefix(strings.TrimSpace(lines[n-1]), "```") {
			lines = lines[:n-1]
		}
		filter = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	if strings.HasPrefix(filter, "ERROR:") {
		return "", errors.New(strings.TrimSpace(strings.TrimPrefix(filter, "ERROR:")))
	}
	if filter == "" {
		return "", errors.New("the LLM returned no filter")
	}
	return filter, nil
}
