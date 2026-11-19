package transformation

import (
	"sync"
	"sync/atomic"

	"github.com/itchyny/gojq"
)

// Transformer applies the latest transformation of a decoder rule to decoded values. The
// transformations of a topic are read once and cached until Invalidate is called.
type Transformer struct {
	store   Store
	cluster string
	// version changes whenever a transformation was invalidated, see Version
	version atomic.Uint64

	mu     sync.Mutex
	topics map[string]map[string]*gojq.Code
}

// NewTransformer returns a transformer for the topics of the given cluster.
func NewTransformer(store Store, cluster string) *Transformer {
	return &Transformer{store: store, cluster: cluster, topics: map[string]map[string]*gojq.Code{}}
}

// Version changes on every invalidation, so results derived from transformations can be cached per version.
func (t *Transformer) Version() uint64 {
	return t.version.Load()
}

// Has reports whether the rule has a transformation.
func (t *Transformer) Has(topic, ruleID string) bool {
	if ruleID == "" {
		return false
	}
	rules, err := t.rules(topic)
	if err != nil {
		return false
	}
	_, ok := rules[ruleID]
	return ok
}

// Transform returns the value transformed by the latest transformation of the rule and whether
// the rule has one. Transformations stored with a filter that no longer compiles are ignored.
func (t *Transformer) Transform(topic, ruleID, value string) (string, bool, error) {
	if ruleID == "" {
		return "", false, nil
	}
	rules, err := t.rules(topic)
	if err != nil {
		return "", false, err
	}
	code, ok := rules[ruleID]
	if !ok {
		return "", false, nil
	}
	out, err := Run(code, value)
	if err != nil {
		return "", false, err
	}
	return out, true, nil
}

// Invalidate drops the cached transformations of the topic, they are read again on next use.
func (t *Transformer) Invalidate(topic string) {
	t.version.Add(1)
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.topics, topic)
}

func (t *Transformer) rules(topic string) (map[string]*gojq.Code, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if rules, ok := t.topics[topic]; ok {
		return rules, nil
	}

	histories, err := t.store.read(t.cluster, topic)
	if err != nil {
		return nil, err
	}
	rules := map[string]*gojq.Code{}
	for ruleID, entries := range histories {
		if len(entries) == 0 {
			continue
		}
		code, err := Compile(entries[len(entries)-1].Filter)
		if err != nil {
			continue
		}
		rules[ruleID] = code
	}
	t.topics[topic] = rules
	return rules, nil
}
