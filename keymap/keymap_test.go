package keymap

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func press(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(text)[0], Text: text}
}

func TestDefaults(t *testing.T) {
	t.Cleanup(Reset)
	Reset()

	assert.Equal(t, []string{"up"}, Keys(TableUp))
	assert.True(t, Matches(tea.KeyPressMsg{Code: tea.KeyUp}, TableUp))
	assert.True(t, Matches(press("d"), RecordsDecoders))
	assert.Empty(t, Keys(TableHalfDown), "half page keys are unbound by default")
}

func TestApplyReplacesOnlyTheGivenActions(t *testing.T) {
	t.Cleanup(Reset)

	require.NoError(t, Apply(map[string][]string{"records.decoders": {"x"}}))

	assert.True(t, Matches(press("x"), RecordsDecoders))
	assert.False(t, Matches(press("d"), RecordsDecoders))
	assert.Equal(t, []string{"up"}, Keys(TableUp), "unlisted actions keep their defaults")
}

func TestApplyEmptyListUnbinds(t *testing.T) {
	t.Cleanup(Reset)

	require.NoError(t, Apply(map[string][]string{"records.decoders": {}}))

	assert.False(t, Matches(press("d"), RecordsDecoders))
	assert.Empty(t, Hint(RecordsDecoders))
}

func TestApplyRejectsInvalidOverrides(t *testing.T) {
	t.Cleanup(Reset)

	assert.ErrorContains(t, Apply(map[string][]string{"records.nope": {"x"}}), `unknown action "records.nope"`)
	assert.ErrorContains(t, Apply(map[string][]string{"records.decoders": {""}}), "invalid key")
	assert.ErrorContains(t, Apply(map[string][]string{"records.decoders": {"ctrl+ x"}}), "invalid key")
	assert.ErrorContains(t, Apply(map[string][]string{"records.decoders": {"ctrl+c"}}), "cannot be bound")

	// a rejected override leaves the active keys untouched
	assert.Equal(t, []string{"d"}, Keys(RecordsDecoders))
}

func TestApplyRejectsClashesInTheSameScope(t *testing.T) {
	t.Cleanup(Reset)

	err := Apply(map[string][]string{"table.half_page_down": {"up"}})
	assert.ErrorContains(t, err, `key "up" is bound to both`)

	assert.NoError(t, Apply(map[string][]string{"records.decoders": {"x"}}),
		"keys that no other action binds are fine")
}

func TestOverridesClashWithTableKeys(t *testing.T) {
	t.Cleanup(Reset)

	// the table is active on the pages, so a page action must not take its keys
	err := Apply(map[string][]string{"records.decoders": {"up"}})
	assert.ErrorContains(t, err, `key "up" is bound to both records.decoders and table.up`)
}

func TestOverlappingDefaultsAreAllowed(t *testing.T) {
	t.Cleanup(Reset)
	// pages bind the same key to different actions in different modes
	enterA := Define("test.enter_a", "test-scope", "enter")
	Define("test.enter_b", "test-scope", "enter")

	require.NoError(t, Apply(nil))
	assert.Equal(t, []string{"enter"}, Keys(enterA))
}

func TestHint(t *testing.T) {
	t.Cleanup(Reset)

	assert.Equal(t, "↑", Hint(TableUp))

	require.NoError(t, Apply(map[string][]string{
		"records.decoders": {"ctrl+s", "ctrl+o"},
		"table.top":        {"left", "right"},
	}))
	assert.Equal(t, "C-s/C-o", Hint(RecordsDecoders))
	assert.Equal(t, "F2", Hint(RecordsEndConsume))
	assert.Equal(t, "←/→", Hint(TableTop))
}
