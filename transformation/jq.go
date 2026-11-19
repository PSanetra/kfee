package transformation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/itchyny/gojq"
)

// Compile parses and compiles a jq filter.
func Compile(filter string) (*gojq.Code, error) {
	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}
	return code, nil
}

// Run applies the compiled filter to the JSON document and returns the first result as indented JSON.
func Run(code *gojq.Code, document string) (string, error) {
	// UseNumber keeps numbers exactly as they were decoded
	decoder := json.NewDecoder(bytes.NewReader([]byte(document)))
	decoder.UseNumber()
	var input any
	if err := decoder.Decode(&input); err != nil {
		return "", fmt.Errorf("document is not JSON: %w", err)
	}

	value, ok := code.Run(input).Next()
	if !ok {
		return "", errors.New("filter produced no output")
	}
	if err, isErr := value.(error); isErr {
		return "", err
	}
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Transform applies the jq filter to the JSON document and returns the first result as indented JSON.
func Transform(filter string, document string) (string, error) {
	code, err := Compile(filter)
	if err != nil {
		return "", err
	}
	return Run(code, document)
}
