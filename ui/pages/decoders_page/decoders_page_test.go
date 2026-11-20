package decoders_page

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kfee/config"
	"kfee/kontext"
	"kfee/tests"
	"kfee/transformation"
	"kfee/ui/clipper"
	"kfee/ui/tabs"
)

func TestDecodersPage(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := &config.Config{Clusters: []config.Cluster{{Name: "prd", Active: true, Protobuf: &config.ProtobufConfig{
		Topics: []config.ProtobufTopicConfig{{Topic: "orders", Rules: []config.ProtobufRule{
			{ID: "r1", MessageType: "acme.OrderCancelled", Header: "type", ValueRegex: "^OrderCancelled$"},
			{ID: "r2", MessageType: "acme.OrderCreated"},
		}}},
	}}}}
	store := transformation.Store{Dir: config.TransformationsDir()}
	_, err := store.Append("prd", "orders", "r1", transformation.Entry{Source: transformation.SourceManual, Filter: "{first}"})
	require.NoError(t, err)
	_, err = store.Append("prd", "orders", "r1", transformation.Entry{Source: transformation.SourceLLM, Filter: "{latest: .id}"})
	require.NoError(t, err)

	ktx := kontext.New(nil)
	ktx.WindowWidth, ktx.WindowHeight, ktx.AvailableHeight = 120, 40, 36
	navigator := tabs.NewMockTopicsTabNavigator()
	m := New("orders", cfg, nil, nil, navigator, map[string]string{"avro:orders-value": `{"type":"record","name":"Order","fields":[{"name":"id","type":"string"}]}`}, clipper.NewMock())

	t.Run("lists the decoders with the latest transformation", func(t *testing.T) {
		render := ansi.Strip(m.View(ktx, tests.Renderer))

		assert.Contains(t, render, "Decoders of orders")
		assert.Contains(t, render, "acme.OrderCancelled")
		assert.Contains(t, render, "{latest: .id}")
		assert.Contains(t, render, "acme.OrderCreated")
		assert.Contains(t, render, "Avro schema of orders-value")
	})

	t.Run("enter shows the schema and the transformations side by side, latest first", func(t *testing.T) {
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		render := ansi.Strip(m.View(ktx, tests.Renderer))

		assert.Contains(t, render, "Schema")
		assert.Contains(t, render, "Unable to describe the schema", "the protobuf schema is shown on the left")
		assert.Contains(t, render, "▶")
		assert.Contains(t, render, "{latest: .id}")
		assert.Contains(t, render, "{first}")
		assert.Less(t, strings.Index(render, "{latest: .id}"), strings.Index(render, "{first}"))
	})

	t.Run("left and right switch between the schema and the transformations", func(t *testing.T) {
		m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		assert.False(t, m.focusSchema)
		m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		assert.True(t, m.focusSchema)
		m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	})

	t.Run("activating an older transformation moves it to the latest position", func(t *testing.T) {
		// the transformations are focused: the cursor moves to the older entry and activates it
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
		render := ansi.Strip(m.View(ktx, tests.Renderer))

		assert.Less(t, strings.Index(render, "{first}"), strings.Index(render, "{latest: .id}"))
		assert.Len(t, strings.Split(render, "{first}"), 2, "the old entry is removed, only one is left")

		// enter activates as well: the latest entry is "{first}" now, so the cursor is on the older one
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		render = ansi.Strip(m.View(ktx, tests.Renderer))
		assert.Less(t, strings.Index(render, "{latest: .id}"), strings.Index(render, "{first}"))
	})

	t.Run("shows the Avro schema next to its transformations", func(t *testing.T) {
		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		// the Avro subject is the third row, after the two protobuf rules
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		render := ansi.Strip(m.View(ktx, tests.Renderer))

		assert.Contains(t, render, `"Order"`)
		assert.Contains(t, render, "Avro schema of orders-value")
	})

	t.Run("esc returns to the decoder list, then to the records", func(t *testing.T) {
		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		assert.Contains(t, ansi.Strip(m.View(ktx, tests.Renderer)), "Decoders of orders")

		cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		require.NotNil(t, cmd)
		assert.IsType(t, tabs.ToCachedConsumptionPageCalledMsg{}, cmd())
	})
}

func TestDecodersPageScrollsTheSchema(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var fields []string
	for i := range 60 {
		fields = append(fields, fmt.Sprintf(`{"name":"field%02d","type":"string"}`, i))
	}
	schema := `{"type":"record","name":"Long","fields":[` + strings.Join(fields, ",") + `]}`
	ktx := kontext.New(nil)
	ktx.WindowWidth, ktx.WindowHeight, ktx.AvailableHeight = 120, 40, 36
	m := New("orders", &config.Config{Clusters: []config.Cluster{{Name: "prd", Active: true}}}, nil, nil,
		tabs.NewMockTopicsTabNavigator(), map[string]string{"avro:orders-value": schema}, clipper.NewMock())

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	render := ansi.Strip(m.View(ktx, tests.Renderer))
	assert.Contains(t, render, `"field00"`)
	assert.NotContains(t, render, `"field59"`, "the schema does not fit, the end is scrolled to")

	for range 400 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	render = ansi.Strip(m.View(ktx, tests.Renderer))
	assert.Contains(t, render, `"field59"`)
	assert.NotContains(t, render, `"field00"`)
}
