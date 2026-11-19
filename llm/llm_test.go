package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kfee/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
		err      string
	}{
		{"plain filter", "{id: .id}\n", "{id: .id}", ""},
		{"code fence", "```jq\n.status |= .\n```", ".status |= .", ""},
		{"error reply", "ERROR: no status field", "", "no status field"},
		{"empty", "  \n", "", "no filter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.output)

			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, filter)
		})
	}
}

func TestBuildPromptIncludesOnlyGivenParts(t *testing.T) {
	prompt := BuildPrompt(Request{Description: "show the status name", Schema: "message A {}"})

	assert.Contains(t, prompt, "Reply with the jq filter only")
	assert.Contains(t, prompt, "## Schema\nmessage A {}")
	assert.Contains(t, prompt, "show the status name")
	assert.NotContains(t, prompt, "## Sample input")
	assert.NotContains(t, prompt, "## Attached file")

	withSample := BuildPrompt(Request{Description: "d", Schema: "s", Sample: `{"id": 1}`,
		Attachments: []Attachment{{Path: "codes.txt", Content: "1 = NEW"}}})
	assert.Contains(t, withSample, "## Sample input\n{\"id\": 1}")
	assert.Contains(t, withSample, "## Attached file codes.txt\n1 = NEW")
}

func TestReadAttachments(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.txt")
	second := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(first, []byte("1 = NEW"), 0o644))
	require.NoError(t, os.WriteFile(second, []byte("2 = DONE"), 0o644))

	attachments, err := ReadAttachments([]string{first, " ", second})

	require.NoError(t, err)
	require.Len(t, attachments, 2)
	assert.Equal(t, "2 = DONE", attachments[1].Content)

	_, err = ReadAttachments([]string{filepath.Join(dir, "missing.txt")})
	assert.Error(t, err)
}

func TestRun(t *testing.T) {
	t.Run("writes the prompt to stdin", func(t *testing.T) {
		out, err := Run(context.Background(), config.LLMConfig{Command: []string{"cat"}}, "hello")

		require.NoError(t, err)
		assert.Equal(t, "hello", out)
	})

	t.Run("passes the prompt as the last argument", func(t *testing.T) {
		out, err := Run(context.Background(), config.LLMConfig{Command: []string{"echo", "-n", "x"}, PromptAsArgument: true}, "hello")

		require.NoError(t, err)
		assert.Equal(t, "x hello", out)
	})

	t.Run("requires a command", func(t *testing.T) {
		_, err := Run(context.Background(), config.LLMConfig{}, "hello")

		assert.ErrorContains(t, err, "no LLM command")
	})

	t.Run("reports a failing command", func(t *testing.T) {
		_, err := Run(context.Background(), config.LLMConfig{Command: []string{"false"}}, "hello")

		assert.Error(t, err)
	})
}
