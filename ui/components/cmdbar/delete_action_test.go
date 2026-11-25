package cmdbar

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kfee/keymap"
	"kfee/tests"
)

// A rebinding in keymap.yaml must move the key that toggles the delete bar, not leave it on the old key.
func TestDeleteBarFollowsRebindingOfItsAction(t *testing.T) {
	t.Cleanup(keymap.Reset)
	require.NoError(t, keymap.Apply(map[string][]string{"test.delete_f4": {"ctrl+x"}}))

	cmdBar := NewDeleteCmdBar[string](nil, func(string) tea.Cmd { return nil },
		WithDeleteAction[string](testDeleteF4))

	cmdBar.Update(tests.Key(tea.KeyF4))
	assert.False(t, cmdBar.IsFocussed(), "the old key no longer toggles the bar")

	cmdBar.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	assert.True(t, cmdBar.IsFocussed(), "the rebound key toggles the bar")
}
