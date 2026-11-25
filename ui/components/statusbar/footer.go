package statusbar

import (
	"strings"

	lg "charm.land/lipgloss/v2"
	"kfee/styles"
)

const footerSeparator = "  "

// Footer lays out the shortcuts in one line of the given width. It shows as many shortcuts as fit. When some
// do not fit, the line ends with helpKey and "all keys", which opens the help listing all of them.
func Footer(shortcuts []Shortcut, width int, helpKey string) string {
	var items []string
	for _, s := range shortcuts {
		if s.Keybinding == "" {
			continue
		}
		items = append(items, styles.Help.Key.Render(s.Keybinding)+" "+styles.Help.Desc.Render(s.Name))
	}

	all := strings.Join(items, footerSeparator)
	if lg.Width(all) <= width {
		return all
	}

	more := styles.Help.Key.Render(helpKey) + " " + styles.Help.Desc.Render("all keys")
	for n := len(items) - 1; n >= 0; n-- {
		line := strings.Join(items[:n], footerSeparator)
		if n > 0 {
			line += footerSeparator
		}
		line += more
		if lg.Width(line) <= width {
			return line
		}
	}
	return lg.NewStyle().MaxWidth(width).Render(more)
}
