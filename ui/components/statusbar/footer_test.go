package statusbar

import (
	"testing"

	lg "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

var footerShortcuts = []Shortcut{
	{Name: "Switch Tabs", Keybinding: "C-left/C-right"},
	{Name: "Search", Keybinding: "/"},
	{Name: "Copy Record", Keybinding: "c"},
	{Name: "Refresh", Keybinding: "F5"},
	{Name: "Unbound", Keybinding: ""},
}

func TestFooterShowsAllShortcutsThatFit(t *testing.T) {
	line := ansi.Strip(Footer(footerShortcuts, 200, "F1"))

	assert.Contains(t, line, "Switch Tabs")
	assert.Contains(t, line, "Copy Record")
	assert.Contains(t, line, "Refresh")
	assert.NotContains(t, line, "all keys", "nothing is left out, so no hint to the help")
	assert.NotContains(t, line, "Unbound", "shortcuts without keys are not shown")
}

func TestFooterPointsToHelpWhenShortcutsAreLeftOut(t *testing.T) {
	width := 60
	line := Footer(footerShortcuts, width, "F1")

	plain := ansi.Strip(line)
	assert.Contains(t, plain, "F1 all keys")
	assert.Contains(t, plain, "Switch Tabs", "the first shortcuts stay visible")
	assert.NotContains(t, plain, "Refresh")
	assert.LessOrEqual(t, lg.Width(line), width)
}

func TestFooterStaysWithinNarrowWidth(t *testing.T) {
	line := Footer(footerShortcuts, 5, "F1")

	assert.LessOrEqual(t, lg.Width(line), 5)
	assert.NotContains(t, line, "\n")
}
