package record_details_page

import (
	"kfee/config"
	"kfee/kadmin"
	"kfee/serdes"
	"kfee/tests"
	"kfee/transformation"
	"kfee/ui/clipper"
	"kfee/ui/tabs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newGenerationPage(t *testing.T, llm *config.LLMConfig, ruleID string) (*Model, transformation.Store, *config.Config) {
	t.Helper()
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc is not installed")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	protoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(protoDir, "orders.proto"), []byte(ordersProto), 0o644))
	cfg := &config.Config{
		Clusters: []config.Cluster{{Name: "prd", Active: true}},
		LLM:      llm,
	}
	config.NewInMemoryConfigIO(cfg)
	ktx := tests.NewKontext(tests.WithConfig(cfg))
	ktx.SetProtoDecoders(serdes.NewProtobufDecoders(&config.ProtobufConfig{Topics: []config.ProtobufTopicConfig{{
		Topic:    "orders",
		ProtoDir: protoDir,
		Rules:    []config.ProtobufRule{{ID: ruleID, MessageType: "acme.orders.v1.OrderCancelled"}},
	}}}))

	record := protoRecord()
	record.Payload = serdes.DesData{
		Value:       `{"id": "o-1", "reason": "late"}`,
		MessageType: "acme.orders.v1.OrderCancelled",
		RuleID:      ruleID,
	}
	m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
	m.View(tests.Kontext, tests.Renderer)
	return m, transformation.Store{Dir: config.TransformationsDir(), Limit: 10}, cfg
}

func TestRecordDetailsPageGenerateTransformation(t *testing.T) {
	fakeLLM := &config.LLMConfig{Command: []string{"sh", "-c", "printf '```jq\\n{kind: .reason}\\n```'"}}

	t.Run("generates a filter into the editor and saves it as generated", func(t *testing.T) {
		m, store, _ := newGenerationPage(t, fakeLLM, "r1")
		m.Update(tests.Key('e'))
		m.transformEditor.description = "show the reason as kind"

		cmd := m.Update(tests.Ctrl('g'))
		require.NotNil(t, cmd)
		assert.True(t, m.transformEditor.generating)
		m.Update(cmd())

		assert.False(t, m.transformEditor.generating)
		assert.Equal(t, "{kind: .reason}", m.transformEditor.filter)

		m.Update(tests.Ctrl('s'))
		history, err := store.Load("prd", "orders", "r1")
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.Equal(t, transformation.SourceLLM, history[0].Source)
		assert.Equal(t, "{kind: .reason}", history[0].Filter)
		assert.Contains(t, ansi.Strip(m.View(tests.Kontext, tests.Renderer)), `"kind": "late"`)
	})

	t.Run("a manually changed generated filter is saved as manual", func(t *testing.T) {
		m, store, _ := newGenerationPage(t, fakeLLM, "r2")
		m.Update(tests.Key('e'))
		m.transformEditor.description = "show the reason as kind"
		cmd := m.Update(tests.Ctrl('g'))
		m.Update(cmd())
		m.transformEditor.filter = `{kind: "changed"}`

		m.Update(tests.Ctrl('s'))

		history, err := store.Load("prd", "orders", "r2")
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.Equal(t, transformation.SourceManual, history[0].Source)
	})

	t.Run("reports a reply that is an error", func(t *testing.T) {
		errorLLM := &config.LLMConfig{Command: []string{"sh", "-c", "echo 'ERROR: no reason field'"}}
		m, _, _ := newGenerationPage(t, errorLLM, "r3")
		m.Update(tests.Key('e'))
		m.transformEditor.description = "show the reason"
		cmd := m.Update(tests.Ctrl('g'))
		m.Update(cmd())

		assert.Contains(t, m.transformEditor.status, "no reason field")
		assert.NotNil(t, m.transformEditor)
	})

	t.Run("asks for the LLM tool when none is configured", func(t *testing.T) {
		m, _, cfg := newGenerationPage(t, nil, "r4")
		m.Update(tests.Key('e'))
		m.transformEditor.description = "show the reason"

		m.Update(tests.Ctrl('g'))
		require.NotNil(t, m.transformEditor.picker)
		m.Update(tests.Key(tea.KeyDown))
		m.Update(tests.Key(tea.KeyEnter))

		assert.Nil(t, m.transformEditor.picker)
		require.NotNil(t, cfg.LLM)
		assert.Equal(t, config.LLMPresets[1].Config.Command, cfg.LLM.Command)
	})

	t.Run("requires a description", func(t *testing.T) {
		m, _, _ := newGenerationPage(t, fakeLLM, "r5")
		m.Update(tests.Key('e'))

		cmd := m.Update(tests.Ctrl('g'))

		assert.Nil(t, cmd)
		assert.Equal(t, "Describe the transformation first.", m.transformEditor.status)
	})
}
