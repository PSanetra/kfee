package config

import (
	"crypto/rand"
	"encoding/hex"
)

// ProtobufConfig configures decoding of raw protobuf records (without schema registry framing).
type ProtobufConfig struct {
	// Protoc is the protoc binary used to compile .proto files, defaults to "protoc" from the PATH.
	Protoc string                `yaml:"protoc,omitempty"`
	Topics []ProtobufTopicConfig `yaml:"topics"`
}

// ProtocBinary returns the configured protoc binary or "protoc" when none is configured.
func (p *ProtobufConfig) ProtocBinary() string {
	if p == nil || p.Protoc == "" {
		return "protoc"
	}
	return p.Protoc
}

// Topic returns the decoder configuration of the given topic or nil if there is none.
func (p *ProtobufConfig) Topic(topic string) *ProtobufTopicConfig {
	if p == nil {
		return nil
	}
	for i := range p.Topics {
		if p.Topics[i].Topic == topic {
			return &p.Topics[i]
		}
	}
	return nil
}

// ProtobufTopicConfig configures how the records of a topic are decoded.
type ProtobufTopicConfig struct {
	Topic string `yaml:"topic"`
	// ProtoDir is the directory containing the .proto files, it is used as import path.
	// It is not needed when all rules decode without schema.
	ProtoDir string `yaml:"protoDir,omitempty"`
	// Rules select the message type of a record, the first matching rule wins.
	Rules []ProtobufRule `yaml:"rules"`
}

// ProtobufRule decodes records as MessageType, or without schema when Raw is set. When Header
// is set, the rule only applies to records having that header with a value matching ValueRegex,
// otherwise it applies to all records.
type ProtobufRule struct {
	// ID identifies the rule across edits, it keeps the transformation history of the rule.
	// It is assigned when the topic configuration is stored.
	ID          string `yaml:"id,omitempty"`
	MessageType string `yaml:"messageType,omitempty"`
	// Raw decodes without schema, showing field numbers instead of names
	Raw        bool   `yaml:"raw,omitempty"`
	Header     string `yaml:"header,omitempty"`
	ValueRegex string `yaml:"valueRegex,omitempty"`
}

func (r ProtobufRule) IsFallback() bool {
	return r.Header == ""
}

// EnsureRuleIDs assigns an ID to every rule without one.
func (t *ProtobufTopicConfig) EnsureRuleIDs() {
	for i := range t.Rules {
		if t.Rules[i].ID == "" {
			t.Rules[i].ID = newRuleID()
		}
	}
}

func newRuleID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// WithRule returns a copy of the topic config including the given rule.
//
// A rule for the same header and value regex is replaced, a fallback rule replaces the
// existing fallback. Header rules are kept in front of the fallback rule. The replaced
// rule keeps its ID when the given rule has none.
func (t ProtobufTopicConfig) WithRule(rule ProtobufRule) ProtobufTopicConfig {
	if rule.ID == "" {
		for _, r := range t.Rules {
			if r.IsFallback() == rule.IsFallback() &&
				(r.IsFallback() || (r.Header == rule.Header && r.ValueRegex == rule.ValueRegex)) {
				rule.ID = r.ID
				break
			}
		}
	}
	var (
		headerRules []ProtobufRule
		fallback    *ProtobufRule
		replaced    bool
	)
	for _, r := range t.Rules {
		switch {
		case r.IsFallback():
			if !rule.IsFallback() {
				fallback = &r
			}
		case r.Header == rule.Header && r.ValueRegex == rule.ValueRegex:
			headerRules = append(headerRules, rule)
			replaced = true
		default:
			headerRules = append(headerRules, r)
		}
	}

	if rule.IsFallback() {
		fallback = &rule
	} else if !replaced {
		headerRules = append(headerRules, rule)
	}

	t.Rules = headerRules
	if fallback != nil {
		t.Rules = append(t.Rules, *fallback)
	}
	return t
}

// SetProtobufTopic stores the protobuf decoder configuration of a topic of the given cluster.
func (c *Config) SetProtobufTopic(clusterName string, topic ProtobufTopicConfig) {
	cluster := c.findCluster(clusterName)
	if cluster == nil {
		return
	}
	topic.EnsureRuleIDs()
	if cluster.Protobuf == nil {
		cluster.Protobuf = &ProtobufConfig{}
	}
	if existing := cluster.Protobuf.Topic(topic.Topic); existing != nil {
		*existing = topic
	} else {
		cluster.Protobuf.Topics = append(cluster.Protobuf.Topics, topic)
	}
	c.flush()
}

// DeleteProtobufTopic removes the protobuf decoder configuration of a topic of the given cluster.
func (c *Config) DeleteProtobufTopic(clusterName string, topic string) {
	cluster := c.findCluster(clusterName)
	if cluster == nil || cluster.Protobuf == nil {
		return
	}
	var topics []ProtobufTopicConfig
	for _, t := range cluster.Protobuf.Topics {
		if t.Topic != topic {
			topics = append(topics, t)
		}
	}
	cluster.Protobuf.Topics = topics
	c.flush()
}

// ensureRuleIDs assigns IDs to all rules without one, reports whether any was assigned.
func (c *Config) ensureRuleIDs() bool {
	changed := false
	for i := range c.Clusters {
		if c.Clusters[i].Protobuf == nil {
			continue
		}
		for j := range c.Clusters[i].Protobuf.Topics {
			topic := &c.Clusters[i].Protobuf.Topics[j]
			for _, r := range topic.Rules {
				if r.ID == "" {
					changed = true
				}
			}
			topic.EnsureRuleIDs()
		}
	}
	return changed
}

func (c *Config) findCluster(name string) *Cluster {
	for i := range c.Clusters {
		if c.Clusters[i].Name == name {
			return &c.Clusters[i]
		}
	}
	return nil
}
