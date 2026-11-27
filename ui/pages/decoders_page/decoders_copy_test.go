package decoders_page

import (
	"errors"
	"kfee/config"
	"kfee/tests"
	"kfee/ui/clipper"
	"kfee/ui/tabs"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodersPageCopiesTheSchema(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := &config.Config{Clusters: []config.Cluster{{Name: "prd", Active: true}}}
	schema := `{"type":"record","name":"Order","fields":[{"name":"id","type":"string"}]}`

	t.Run("c copies the schema of the decoder under the cursor", func(t *testing.T) {
		var copied string
		clip := &clipper.MockClipper{WriteFunc: func(text string) error {
			copied = text
			return nil
		}}
		m := New("orders", cfg, nil, nil, tabs.NewMockTopicsTabNavigator(), map[string]string{"avro:orders-value": schema}, clip)

		cmd := m.Update(tests.Key('c'))

		require.NotNil(t, cmd)
		assert.Contains(t, copied, `"name": "Order"`)
		assert.NotContains(t, copied, "\x1b", "the copied schema has no styling")
	})

	t.Run("c copies the schema in the detail view too", func(t *testing.T) {
		var copied string
		clip := &clipper.MockClipper{WriteFunc: func(text string) error {
			copied = text
			return nil
		}}
		m := New("orders", cfg, nil, nil, tabs.NewMockTopicsTabNavigator(), map[string]string{"avro:orders-value": schema}, clip)
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		require.True(t, m.detail)

		m.Update(tests.Key('c'))

		assert.Contains(t, copied, `"name": "Order"`)
	})

	t.Run("a failed copy is reported", func(t *testing.T) {
		clip := &clipper.MockClipper{WriteFunc: func(string) error { return errors.New("no clipboard") }}
		m := New("orders", cfg, nil, nil, tabs.NewMockTopicsTabNavigator(), map[string]string{"avro:orders-value": schema}, clip)

		msg := m.copySchema(m.selected())()

		copyErr, ok := msg.(CopyErrorMsg)
		require.True(t, ok, "got %T", msg)
		assert.EqualError(t, copyErr.Err, "no clipboard")
	})

	t.Run("c does nothing without decoders", func(t *testing.T) {
		var copied string
		clip := &clipper.MockClipper{WriteFunc: func(text string) error {
			copied = text
			return nil
		}}
		m := New("orders", cfg, nil, nil, tabs.NewMockTopicsTabNavigator(), nil, clip)

		m.Update(tests.Key('c'))

		assert.Empty(t, copied)
	})
}
