// Package help renders the keybindings modal shown on the help key.
package help

import (
	"fmt"
	"kfee/keymap"
	"kfee/styles"
	"kfee/ui/components/statusbar"
	"kfee/ui/components/tab"
	"strings"

	lg "charm.land/lipgloss/v2"
)

// ActionClose closes the help modal. It is the only key handled while the modal is open.
var ActionClose = keymap.Define("help.close", "help", "esc", "q")

// chromeHeight is the number of lines the modal uses besides the shortcut rows:
// border (2), vertical padding (2), title + blank line (2), blank line + footer (2).
const chromeHeight = 8

const columnGap = "    "

// View renders the keybindings modal. Shortcuts are laid out in as many
// columns as needed to keep the modal within maxHeight.
func View(title string, shortcuts []statusbar.Shortcut, maxHeight int) string {
	heading := "Keybindings"
	if title != "" {
		heading += " · " + title
	}

	rowsPerColumn := max(1, maxHeight-chromeHeight)
	var columns []string
	for start := 0; start < len(shortcuts); start += rowsPerColumn {
		end := min(start+rowsPerColumn, len(shortcuts))
		columns = append(columns, renderColumn(shortcuts[start:end]))
	}

	var body []string
	for i, c := range columns {
		if i > 0 {
			body = append(body, columnGap)
		}
		body = append(body, c)
	}

	return styles.Help.Modal.Render(lg.JoinVertical(lg.Left,
		styles.Help.Title.Render(heading),
		"",
		lg.JoinHorizontal(lg.Top, body...),
		"",
		styles.Help.Footer.Render(fmt.Sprintf("%s / %s close", keymap.Hint(ActionClose), keymap.Hint(tab.ActionHelp))),
	))
}

func renderColumn(shortcuts []statusbar.Shortcut) string {
	keyWidth := 0
	for _, s := range shortcuts {
		keyWidth = max(keyWidth, lg.Width(s.Keybinding))
	}

	rows := make([]string, len(shortcuts))
	for i, s := range shortcuts {
		pad := strings.Repeat(" ", keyWidth-lg.Width(s.Keybinding))
		rows[i] = pad + styles.Help.Key.Render(s.Keybinding) + "  " + styles.Help.Desc.Render(s.Name)
	}
	return lg.JoinVertical(lg.Left, rows...)
}
