package transformation

import (
	"github.com/itchyny/gojq"
)

// Format lays out a jq filter for reading: each pipe of the top level starts a new line. Everything
// else is written in its canonical form on one line, and a filter that does not parse is returned as it is.
func Format(filter string) string {
	query, err := gojq.Parse(filter)
	if err != nil {
		return filter
	}
	return formatPipes(query)
}

func formatPipes(q *gojq.Query) string {
	if q.Meta != nil || len(q.Imports) > 0 || len(q.FuncDefs) > 0 || len(q.Patterns) > 0 ||
		q.Op != gojq.OpPipe || q.Left == nil || q.Right == nil {
		return q.String()
	}
	return formatPipes(q.Left) + "\n| " + formatPipes(q.Right)
}
