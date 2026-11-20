package record_details_page

import (
	"kfee/config"
	"kfee/kadmin"
	"kfee/serdes"
	"kfee/tests"
	"kfee/transformation"
	"kfee/ui/clipper"
	"kfee/ui/tabs"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordDetailsPageTransformEditor(t *testing.T) {
	newPage := func(t *testing.T, ruleID string) (*Model, transformation.Store) {
		t.Helper()
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		cfg := &config.Config{Clusters: []config.Cluster{{Name: "prd", Active: true}}}
		config.NewInMemoryConfigIO(cfg)
		ktx := tests.NewKontext(tests.WithConfig(cfg))
		ktx.SetProtoDecoders(serdes.NewProtobufDecoders(nil))

		record := protoRecord()
		record.Payload = serdes.DesData{
			Value:       `{"id": "o-1", "reason": "late"}`,
			MessageType: "acme.orders.v1.OrderCancelled",
			RuleID:      ruleID,
		}
		m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
		m.View(tests.Kontext, tests.Renderer)
		return m, transformation.Store{Dir: config.TransformationsDir(), Limit: 10}
	}

	t.Run("saves the filter to the history of the rule", func(t *testing.T) {
		m, store := newPage(t, "r1")

		m.Update(tests.Key('e'))
		require.NotNil(t, m.transformEditor)
		assert.Equal(t, ".", m.transformEditor.filter)

		m.transformEditor.filter = `{order: .id, kind: "cancelled"}`
		m.Update(tests.Ctrl('s'))

		assert.Nil(t, m.transformEditor)
		history, err := store.Load("prd", "orders", "r1")
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.Equal(t, transformation.SourceManual, history[0].Source)
		assert.Equal(t, `{order: .id, kind: "cancelled"}`, history[0].Filter)
		render := ansi.Strip(m.View(tests.Kontext, tests.Renderer))
		assert.Contains(t, render, `"kind": "cancelled"`)
		assert.Contains(t, render, "[ transformed ]")
	})

	t.Run("does not save a filter that fails on the record", func(t *testing.T) {
		m, store := newPage(t, "r2")

		m.Update(tests.Key('e'))
		m.transformEditor.filter = `.id + 1`
		m.Update(tests.Ctrl('s'))

		assert.NotNil(t, m.transformEditor, "editor stays open")
		history, err := store.Load("prd", "orders", "r2")
		require.NoError(t, err)
		assert.Empty(t, history)
	})

	t.Run("esc closes the editor without saving", func(t *testing.T) {
		m, store := newPage(t, "r3")

		m.Update(tests.Key('e'))
		m.transformEditor.filter = `{kind: "x"}`
		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

		assert.Nil(t, m.transformEditor)
		history, err := store.Load("prd", "orders", "r3")
		require.NoError(t, err)
		assert.Empty(t, history)
	})

	t.Run("refuses records not decoded by a rule", func(t *testing.T) {
		m, _ := newPage(t, "")

		m.Update(tests.Key('e'))

		assert.Nil(t, m.transformEditor)
	})
}
