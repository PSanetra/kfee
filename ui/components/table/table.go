package table

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"kfee/keymap"
	"kfee/styles"
)

// NewDefaultTable creates a focused table with the theme's styles.
func NewDefaultTable() table.Model {
	return NewTable(table.WithFocused(true), table.WithStyles(styles.Table.Styles))
}

// NewTable creates a table with the key bindings of the keymap package. Every table of kfee is created
// through here, so the keys of the table actions apply everywhere.
func NewTable(opts ...table.Option) table.Model {
	return table.New(append([]table.Option{table.WithKeyMap(tableKeyMap())}, opts...)...)
}

// tableKeyMap builds the bindings of the table from the active keys. Actions without keys are unbound.
func tableKeyMap() table.KeyMap {
	bind := func(a keymap.Action, help string) key.Binding {
		return key.NewBinding(key.WithKeys(keymap.Keys(a)...), key.WithHelp(keymap.Hint(a), help))
	}
	return table.KeyMap{
		LineUp:       bind(keymap.TableUp, "up"),
		LineDown:     bind(keymap.TableDown, "down"),
		PageUp:       bind(keymap.TablePageUp, "page up"),
		PageDown:     bind(keymap.TablePageDown, "page down"),
		HalfPageUp:   bind(keymap.TableHalfUp, "½ page up"),
		HalfPageDown: bind(keymap.TableHalfDown, "½ page down"),
		GotoTop:      bind(keymap.TableTop, "go to start"),
		GotoBottom:   bind(keymap.TableBottom, "go to end"),
	}
}

// View renders t with the current theme's table styles.
// Tables copy their styles when created, so re-apply them to follow theme changes.
func View(t *table.Model) string {
	t.SetStyles(styles.Table.Styles)
	return t.View()
}

// RowWidth is the rendered width of a row with the given columns.
// bubbles v2 tables clip rows to their width, which defaults to 0.
func RowWidth(cols []table.Column) int {
	cellPadding := styles.Table.Styles.Cell.GetHorizontalFrameSize()
	w := 0
	for _, c := range cols {
		w += c.Width + cellPadding
	}
	return w
}
