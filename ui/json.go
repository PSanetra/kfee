package ui

import (
	"bytes"
	"encoding/json"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chrome_styles "github.com/alecthomas/chroma/v2/styles"
	"kfee/styles"
	"strings"
)

func PrettyPrintJson(text string) string {
	formatted, lexer := formatPayload(text)
	return highlight(formatted, lexer)
}

// FormatPayload formats the payload like PrettyPrintJson, without highlighting it.
func FormatPayload(text string) string {
	formatted, _ := formatPayload(text)
	return formatted
}

// PrettyPrintJsonLines pretty prints like PrettyPrintJson but highlights only the first maxLines
// lines, which is much faster for large payloads, e.g. for previews.
func PrettyPrintJsonLines(text string, maxLines int) string {
	formatted, lexer := formatPayload(text)
	if lines := strings.SplitN(formatted, "\n", maxLines+1); len(lines) > maxLines {
		formatted = strings.Join(lines[:maxLines], "\n")
	}
	return highlight(formatted, lexer)
}

// formatPayload indents JSON payloads and chooses the lexer for highlighting them.
func formatPayload(text string) (string, chroma.Lexer) {
	if strings.Contains(text, "{") &&
		strings.Contains(text, "}") {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, []byte(text), "", "\t"); err != nil {
			// not JSON after all, highlight it as is
			return text, lexers.Get("json")
		}
		return prettyJSON.String(), lexers.Get("json")
	} else if strings.Contains(text, "/>") {
		return text, lexers.Get("XML")
	}
	return text, lexers.Fallback
}

func highlight(text string, lexer chroma.Lexer) string {
	builder := &strings.Builder{}
	iterator, _ := lexer.Tokenise(nil, text)
	formatters.TTY256.Format(builder, chrome_styles.Get(styles.Current.Syntax), iterator)
	return builder.String()
}
