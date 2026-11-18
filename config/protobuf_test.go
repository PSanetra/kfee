package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestProtobufTopicConfigWithRule(t *testing.T) {
	typeRule := func(value, messageType string) ProtobufRule {
		return ProtobufRule{MessageType: messageType, Header: "type", ValueRegex: value}
	}
	fallback := func(messageType string) ProtobufRule {
		return ProtobufRule{MessageType: messageType}
	}

	tests := []struct {
		name     string
		rules    []ProtobufRule
		rule     ProtobufRule
		expected []ProtobufRule
	}{
		{
			name:     "adds the first rule",
			rule:     fallback("A"),
			expected: []ProtobufRule{fallback("A")},
		},
		{
			name:     "adds header rules in front of the fallback",
			rules:    []ProtobufRule{typeRule("^a$", "A"), fallback("F")},
			rule:     typeRule("^b$", "B"),
			expected: []ProtobufRule{typeRule("^a$", "A"), typeRule("^b$", "B"), fallback("F")},
		},
		{
			name:     "replaces rules of the same header and regex",
			rules:    []ProtobufRule{typeRule("^a$", "A"), typeRule("^b$", "B"), fallback("F")},
			rule:     typeRule("^a$", "C"),
			expected: []ProtobufRule{typeRule("^a$", "C"), typeRule("^b$", "B"), fallback("F")},
		},
		{
			name:     "replaces the fallback",
			rules:    []ProtobufRule{typeRule("^a$", "A"), fallback("F")},
			rule:     fallback("G"),
			expected: []ProtobufRule{typeRule("^a$", "A"), fallback("G")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topic := ProtobufTopicConfig{Topic: "orders", Rules: tt.rules}

			assert.Equal(t, tt.expected, topic.WithRule(tt.rule).Rules)
		})
	}
}

func TestProtobufTopics(t *testing.T) {
	newConfig := func() *Config {
		c := &Config{Clusters: []Cluster{{Name: "prd", Active: true}, {Name: "dev"}}}
		NewInMemoryConfigIO(c)
		return c
	}
	orders := ProtobufTopicConfig{
		Topic:    "orders",
		ProtoDir: "~/protos",
		Rules:    []ProtobufRule{{MessageType: "acme.Order"}},
	}

	t.Run("sets and replaces topics of the given cluster", func(t *testing.T) {
		c := newConfig()

		c.SetProtobufTopic("prd", orders)
		updated := orders
		updated.ProtoDir = "/other"
		c.SetProtobufTopic("prd", updated)

		assert.Equal(t, []ProtobufTopicConfig{updated}, c.Clusters[0].Protobuf.Topics)
		assert.Nil(t, c.Clusters[1].Protobuf)
	})

	t.Run("deletes topics", func(t *testing.T) {
		c := newConfig()
		c.SetProtobufTopic("prd", orders)

		c.DeleteProtobufTopic("prd", "orders")

		assert.Empty(t, c.Clusters[0].Protobuf.Topics)
	})

	t.Run("updating a cluster keeps its protobuf decoders", func(t *testing.T) {
		c := newConfig()
		c.SetProtobufTopic("prd", orders)

		c.RegisterCluster(RegistrationDetails{Name: "prd", Host: "localhost:9092", AuthMethod: AuthMethodNone})

		assert.Equal(t, []ProtobufTopicConfig{orders}, c.Clusters[0].Protobuf.Topics)
	})

	t.Run("yaml round trip", func(t *testing.T) {
		c := newConfig()
		c.SetProtobufTopic("prd", ProtobufTopicConfig{
			Topic:    "orders",
			ProtoDir: "~/protos",
			Rules: []ProtobufRule{
				{MessageType: "acme.OrderCreated", Header: "type", ValueRegex: "^OrderCreated$"},
				{MessageType: "acme.Order"},
			},
		})

		data, err := yaml.Marshal(c.Clusters[0].Protobuf)
		assert.NoError(t, err)

		assert.Equal(t, `topics:
    - topic: orders
      protoDir: ~/protos
      rules:
        - messageType: acme.OrderCreated
          header: type
          valueRegex: ^OrderCreated$
        - messageType: acme.Order
`, string(data))

		data, err = yaml.Marshal(c.Clusters[1])
		assert.NoError(t, err)
		assert.NotContains(t, string(data), "protobuf")
	})
}
