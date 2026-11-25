package table

import (
	"testing"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kfee/keymap"
)

func newTestTable(t *testing.T) table.Model {
	t.Helper()
	t.Cleanup(keymap.Reset)
	rows := make([]table.Row, 30)
	for i := range rows {
		rows[i] = table.Row{"row"}
	}
	return NewTable(
		table.WithColumns([]table.Column{{Title: "c", Width: 10}}),
		table.WithRows(rows),
		table.WithHeight(5),
		table.WithFocused(true),
	)
}

func send(t *testing.T, m table.Model, msg tea.KeyPressMsg) table.Model {
	t.Helper()
	m, _ = m.Update(msg)
	return m
}

func TestDefaultNavigationKeys(t *testing.T) {
	m := newTestTable(t)

	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, m.Cursor())

	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.Greater(t, m.Cursor(), 1, "page down moves by a page")
}

func TestLetterKeysAreUnbound(t *testing.T) {
	m := newTestTable(t)

	for _, text := range []string{"j", "k", "f", "b", "d", "u", "g", "G"} {
		before := m.Cursor()
		m = send(t, m, tea.KeyPressMsg{Code: []rune(text)[0], Text: text})
		assert.Equal(t, before, m.Cursor(), "key %q must not move the table", text)
	}
}

func TestCtrlHalfPageKeysAreUnbound(t *testing.T) {
	m := newTestTable(t)

	m = send(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	assert.Equal(t, 0, m.Cursor())
	m = send(t, m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	assert.Equal(t, 0, m.Cursor())
}

func TestConfiguredKeyMovesTheTable(t *testing.T) {
	// the table reads the keys when it is created, like the tables of a started kfee
	require.NoError(t, keymap.Apply(map[string][]string{"table.page_down": {"ctrl+v"}}))
	m := newTestTable(t)

	m = send(t, m, tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	assert.Greater(t, m.Cursor(), 0)

	before := m.Cursor()
	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.Equal(t, before, m.Cursor(), "the replaced key no longer pages")
}
