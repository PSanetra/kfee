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

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordDetailsPageTransformation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := &config.Config{Clusters: []config.Cluster{{Name: "prd", Active: true}}}
	config.NewInMemoryConfigIO(cfg)
	ktx := tests.NewKontext(tests.WithConfig(cfg))
	ktx.SetProtoDecoders(serdes.NewProtobufDecoders(nil))

	record := protoRecord()
	record.Payload = serdes.DesData{
		Value:       `{"id": "o-1", "reason": "late"}`,
		MessageType: "acme.orders.v1.OrderCancelled",
		RuleID:      "r1",
	}
	store := transformation.Store{Dir: config.TransformationsDir(), Limit: 10}
	_, err := store.Append("prd", "orders", "r1", transformation.Entry{
		Source: transformation.SourceManual,
		Filter: `{order: .id, kind: "cancelled"}`,
	})
	require.NoError(t, err)

	m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
	m.View(tests.Kontext, tests.Renderer)

	t.Run("shows the transformed payload by default", func(t *testing.T) {
		render := ansi.Strip(m.View(tests.Kontext, tests.Renderer))

		assert.Contains(t, render, `"kind": "cancelled"`)
		assert.NotContains(t, render, `"reason"`)
		assert.Contains(t, render, "[ transformed ]")
	})

	t.Run("t shows the original and back", func(t *testing.T) {
		m.Update(tests.Key('t'))
		render := ansi.Strip(m.View(tests.Kontext, tests.Renderer))
		assert.Contains(t, render, `"id": "o-1"`)
		assert.Contains(t, render, "[ original ]")

		m.Update(tests.Key('t'))
		render = ansi.Strip(m.View(tests.Kontext, tests.Renderer))
		assert.Contains(t, render, `"kind": "cancelled"`)
	})
}

func TestRecordDetailsPageWithoutTransformation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := &config.Config{Clusters: []config.Cluster{{Name: "prd", Active: true}}}
	config.NewInMemoryConfigIO(cfg)
	ktx := tests.NewKontext(tests.WithConfig(cfg))
	ktx.SetProtoDecoders(serdes.NewProtobufDecoders(nil))

	record := protoRecord()
	m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
	m.View(tests.Kontext, tests.Renderer)

	render := ansi.Strip(m.View(tests.Kontext, tests.Renderer))
	assert.NotContains(t, render, "[ transformed ]")
	assert.NotContains(t, render, "[ original ]")
	m.Update(tests.Key('t'))
	assert.NotContains(t, ansi.Strip(m.View(tests.Kontext, tests.Renderer)), "[ original ]")
}
