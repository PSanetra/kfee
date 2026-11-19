// Package transformation keeps the history of the transformations applied to the records
// decoded by a protobuf rule.
package transformation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sources of a transformation.
const (
	SourceManual = "manual"
	SourceLLM    = "llm"
)

// Entry is one applied transformation of a rule.
type Entry struct {
	Applied time.Time `json:"applied"`
	// Source is SourceManual when the filter was written by the user, SourceLLM when it was generated.
	Source string `json:"source"`
	// Filter is the jq filter applied to the decoded records.
	Filter string `json:"filter"`
}

// Store persists the transformation history below Dir, one file per cluster and topic.
type Store struct {
	Dir string
	// Limit is the number of entries kept per rule, the oldest are dropped first.
	Limit int
}

// Load returns the history of the rule, oldest first.
func (s Store) Load(cluster, topic, ruleID string) ([]Entry, error) {
	rules, err := s.read(cluster, topic)
	if err != nil {
		return nil, err
	}
	return rules[ruleID], nil
}

// Append adds the entry to the history of the rule and returns the history, oldest first.
func (s Store) Append(cluster, topic, ruleID string, entry Entry) ([]Entry, error) {
	rules, err := s.read(cluster, topic)
	if err != nil {
		return nil, err
	}
	history := append(rules[ruleID], entry)
	if s.Limit > 0 && len(history) > s.Limit {
		history = history[len(history)-s.Limit:]
	}
	rules[ruleID] = history
	if err := s.write(cluster, topic, rules); err != nil {
		return nil, err
	}
	return history, nil
}

// Activate makes the entry at index the current transformation of the rule: it is moved to the
// end of the history with the time of activation. It returns the history, oldest first.
func (s Store) Activate(cluster, topic, ruleID string, index int) ([]Entry, error) {
	rules, err := s.read(cluster, topic)
	if err != nil {
		return nil, err
	}
	old := rules[ruleID]
	if index < 0 || index >= len(old) {
		return nil, fmt.Errorf("no transformation at position %d", index)
	}
	entry := old[index]
	entry.Applied = time.Now()
	history := make([]Entry, 0, len(old))
	history = append(history, old[:index]...)
	history = append(history, old[index+1:]...)
	history = append(history, entry)
	rules[ruleID] = history
	if err := s.write(cluster, topic, rules); err != nil {
		return nil, err
	}
	return history, nil
}

// file is the path of the history file of a cluster and topic.
func (s Store) file(cluster, topic string) string {
	return filepath.Join(s.Dir, escapeName(cluster), escapeName(topic)+".json")
}

// read returns the histories of all rules of the topic, keyed by rule ID.
func (s Store) read(cluster, topic string) (map[string][]Entry, error) {
	rules := map[string][]Entry{}
	data, err := os.ReadFile(s.file(cluster, topic))
	if errors.Is(err, fs.ErrNotExist) {
		return rules, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// write replaces the history file atomically, so a crash never leaves a partial file behind.
func (s Store) write(cluster, topic string, rules map[string][]Entry) error {
	file := s.file(cluster, topic)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// escapeName makes a cluster or topic name usable as a file name. Dots are escaped as well,
// so names like "." and ".." cannot point outside the directory.
func escapeName(name string) string {
	return strings.ReplaceAll(url.PathEscape(name), ".", "%2E")
}
