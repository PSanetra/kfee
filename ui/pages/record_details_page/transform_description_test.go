package record_details_page

import (
	"kfee/config"
	"kfee/kadmin"
	"kfee/serdes"
	"kfee/tests"
	"kfee/ui/clipper"
	"kfee/ui/tabs"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDescriptionPage(t *testing.T) *Model {
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
		RuleID:      "r1",
	}
	m := New(record, "orders", []kadmin.ConsumerRecord{*record}, 0, clipper.NewMock(), ktx, tabs.NewMockTopicsTabNavigator())
	m.View(tests.Kontext, tests.Renderer)
	m.Update(tests.Key('e'))
	require.NotNil(t, m.transformEditor)
	return m
}

func TestTransformDescriptionInput(t *testing.T) {
	t.Run("pasted text is inserted into the description", func(t *testing.T) {
		m := newDescriptionPage(t)

		m.Update(tea.PasteMsg{Content: "show the reason"})

		assert.Equal(t, "show the reason", m.transformEditor.description)
	})

	t.Run("pasted multiline text keeps its lines", func(t *testing.T) {
		m := newDescriptionPage(t)

		m.Update(tea.PasteMsg{Content: "first line\nsecond line"})

		assert.Equal(t, "first line\nsecond line", m.transformEditor.description)
	})

	t.Run("enter inserts a new line and keeps the editor open", func(t *testing.T) {
		m := newDescriptionPage(t)

		m.Update(tests.Key('a'))
		m.Update(tests.Key(tea.KeyEnter))
		m.Update(tests.Key('b'))

		require.NotNil(t, m.transformEditor, "enter must not close or submit the editor")
		assert.Equal(t, "a\nb", m.transformEditor.description)
	})

	t.Run("enter in the filter inserts a new line", func(t *testing.T) {
		m := newDescriptionPage(t)
		if cmd := m.Update(tests.Key(tea.KeyTab)); cmd != nil {
			m.Update(cmd())
		}

		m.Update(tests.Key('.'))
		m.Update(tests.Key(tea.KeyEnter))
		m.Update(tests.Key('x'))

		require.NotNil(t, m.transformEditor, "enter must not close the editor")
		assert.Equal(t, "..\nx", m.transformEditor.filter)
	})

	t.Run("the text of the editor goes into the description only", func(t *testing.T) {
		m := newDescriptionPage(t)
		m.Update(tea.PasteMsg{Content: "old text"})

		m.Update(externalEditedMsg{target: editDescription, text: "from editor"})

		assert.Equal(t, "from editor", m.transformEditor.description)
		assert.Equal(t, ".", m.transformEditor.filter)
	})

	t.Run("the text of the editor goes into the filter only and keeps its focus", func(t *testing.T) {
		m := newDescriptionPage(t)
		m.Update(tea.PasteMsg{Content: "show the reason"})
		if cmd := m.Update(tests.Key(tea.KeyTab)); cmd != nil {
			m.Update(cmd())
		}

		m.Update(externalEditedMsg{target: editFilter, text: `{kind: "late"}`})

		assert.Equal(t, `{kind: "late"}`, m.transformEditor.filter)
		assert.Equal(t, "show the reason", m.transformEditor.description)
		assert.True(t, m.transformEditor.form.GetFocusedField() == m.transformEditor.filterText, "the filter keeps the focus")
	})

	t.Run("the focused field is the one edited", func(t *testing.T) {
		e := newTransformEditor(`{"id": "o-1"}`, ".id")
		e.description = "desc"

		target, text, ok := e.focusedText()
		require.True(t, ok)
		assert.Equal(t, editDescription, target)
		assert.Equal(t, "desc", text)

		e.focusFilter()
		target, text, ok = e.focusedText()
		require.True(t, ok)
		assert.Equal(t, editFilter, target)
		assert.Equal(t, ".id", text)
	})

	t.Run("ctrl+e starts the external editor", func(t *testing.T) {
		m := newDescriptionPage(t)
		m.Update(tests.Key('a'))

		cmd := m.Update(tests.Ctrl('e'))

		require.NotNil(t, cmd, "ctrl+e starts $EDITOR on the description")
		assert.Equal(t, "a", m.transformEditor.description, "the text stays until the editor returns")
	})

	t.Run("paste into the filter", func(t *testing.T) {
		m := newDescriptionPage(t)
		if cmd := m.Update(tests.Key(tea.KeyTab)); cmd != nil {
			m.Update(cmd())
		}

		m.Update(tea.PasteMsg{Content: `{kind: "late"}`})

		assert.Equal(t, `.{kind: "late"}`, m.transformEditor.filter)
		assert.Equal(t, "", m.transformEditor.description)
	})
}
