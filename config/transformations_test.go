package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestResolveDataDir(t *testing.T) {
	home := func() (string, error) { return filepath.FromSlash("/home/user"), nil }

	tests := []struct {
		name        string
		xdgDataHome string
		expected    string
	}{
		{"uses XDG_DATA_HOME", "/xdg/data", "/xdg/data/kfee/transformations"},
		{"defaults to ~/.local/share", "", "/home/user/.local/share/kfee/transformations"},
		{"ignores relative XDG_DATA_HOME", "relative/data", "/home/user/.local/share/kfee/transformations"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, filepath.FromSlash(tt.expected), resolveDataDir(filepath.FromSlash(tt.xdgDataHome), home))
		})
	}
}

func TestWithRuleKeepsIDOfReplacedRule(t *testing.T) {
	topic := ProtobufTopicConfig{Topic: "orders", Rules: []ProtobufRule{
		{ID: "h1", MessageType: "A", Header: "type", ValueRegex: "^a$"},
		{ID: "f1", MessageType: "F"},
	}}

	updated := topic.WithRule(ProtobufRule{MessageType: "C", Header: "type", ValueRegex: "^a$"})
	assert.Equal(t, "h1", updated.Rules[0].ID)

	updated = topic.WithRule(ProtobufRule{MessageType: "G"})
	assert.Equal(t, "f1", updated.Rules[1].ID)

	// a new rule has no ID until the topic configuration is stored
	added := topic.WithRule(ProtobufRule{MessageType: "B", Header: "type", ValueRegex: "^b$"})
	assert.Empty(t, added.Rules[1].ID)
}

func TestEnsureRuleIDsKeepsExistingIDs(t *testing.T) {
	topic := ProtobufTopicConfig{Rules: []ProtobufRule{{ID: "keep"}, {MessageType: "A"}}}

	topic.EnsureRuleIDs()

	assert.Equal(t, "keep", topic.Rules[0].ID)
	assert.NotEmpty(t, topic.Rules[1].ID)
	assert.NotEqual(t, topic.Rules[0].ID, topic.Rules[1].ID)
}

func TestSetProtobufTopicAssignsRuleIDs(t *testing.T) {
	c := &Config{Clusters: []Cluster{{Name: "prd"}}}
	NewInMemoryConfigIO(c)

	c.SetProtobufTopic("prd", ProtobufTopicConfig{Topic: "orders", Rules: []ProtobufRule{{MessageType: "A"}}})

	assert.NotEmpty(t, c.Clusters[0].Protobuf.Topics[0].Rules[0].ID)
}

func TestLLMSettingsRoundTrip(t *testing.T) {
	c := &Config{
		LLM:                   &LLMConfig{Command: []string{"claude", "-p"}, SendSample: true},
		TransformationHistory: 5,
	}

	cfg, _ := splitConfig(c)
	out, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	var parsed configFile
	require.NoError(t, yaml.Unmarshal(out, &parsed))
	loaded := joinConfig(parsed, connectionsFile{})

	assert.Equal(t, c.LLM, loaded.LLM)
	assert.Equal(t, 5, loaded.TransformationHistoryLimit())
}

func TestReadAssignsMissingRuleIDsOnce(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	connectionsPath := filepath.Join(dir, "connections.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`clusters:
    - name: prd
      protobuf:
        topics:
            - topic: orders
              rules:
                - messageType: acme.Order
`), 0o644))
	io := newFileIO(configPath, connectionsPath)

	first, err := io.read()
	require.NoError(t, err)
	id := first.Clusters[0].Protobuf.Topics[0].Rules[0].ID
	require.NotEmpty(t, id)

	second, err := io.read()
	require.NoError(t, err)
	assert.Equal(t, id, second.Clusters[0].Protobuf.Topics[0].Rules[0].ID)
}

func TestLLMDefaults(t *testing.T) {
	c := &Config{}

	assert.False(t, c.LLM.Configured())
	assert.False(t, (&LLMConfig{}).Configured())
	assert.Equal(t, 10, c.TransformationHistoryLimit())
}
