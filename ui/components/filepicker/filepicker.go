// Package filepicker lets the user browse to and choose a single file.
package filepicker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"kfee/keymap"
	"kfee/styles"
)

const defaultHeight = 12

var (
	actionUp       = keymap.Define("filepicker.up", "filepicker", "up", "ctrl+p")
	actionDown     = keymap.Define("filepicker.down", "filepicker", "down", "ctrl+n")
	actionPageUp   = keymap.Define("filepicker.page_up", "filepicker", "pgup")
	actionPageDown = keymap.Define("filepicker.page_down", "filepicker", "pgdown")
	actionOpen     = keymap.Define("filepicker.open", "filepicker", "right", "enter")
	actionParent   = keymap.Define("filepicker.parent", "filepicker", "left")
)

// OpenHint is the keys that open the selected folder, e.g. "→/enter".
func OpenHint() string { return keymap.Hint(actionOpen) }

// ParentHint is the key that goes to the parent folder, e.g. "←".
func ParentHint() string { return keymap.Hint(actionParent) }

type entry struct {
	label  string
	path   string
	isDir  bool
	parent bool
	size   int64
}

// Model browses directories and chooses a file.
type Model struct {
	dir     string
	height  int
	all     []entry
	visible []entry
	cursor  int
	offset  int
	filter  string
	err     error
}

// New starts browsing at start, or its closest existing parent.
func New(start string) *Model {
	m := &Model{height: defaultHeight}
	m.chdir(existingDir(start))
	return m
}

// Dir is the directory currently browsed.
func (m *Model) Dir() string {
	return m.dir
}

// Update handles a key press and returns the chosen file, if the user chose one.
func (m *Model) Update(msg tea.KeyPressMsg) (chosen string) {
	switch {
	case keymap.Matches(msg, actionUp):
		m.move(-1)
	case keymap.Matches(msg, actionDown):
		m.move(1)
	case keymap.Matches(msg, actionPageUp):
		m.move(-m.height)
	case keymap.Matches(msg, actionPageDown):
		m.move(m.height)
	case keymap.Matches(msg, actionOpen):
		if m.cursor < len(m.visible) {
			e := m.visible[m.cursor]
			if e.isDir {
				m.chdir(e.path)
			} else {
				return e.path
			}
		}
	case keymap.Matches(msg, actionParent):
		m.chdir(filepath.Dir(m.dir))
	case msg.String() == "backspace":
		// editing the filter text, the picker goes up a directory when there is none
		if m.filter != "" {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.applyFilter()
		} else {
			m.chdir(filepath.Dir(m.dir))
		}
	default:
		if msg.Text != "" && (msg.Mod == 0 || msg.Mod == tea.ModShift) {
			m.filter += msg.Text
			m.applyFilter()
		}
	}
	return ""
}

func (m *Model) move(delta int) {
	if len(m.visible) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.visible)-1)
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
}

func (m *Model) chdir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.err = err
		return
	}
	m.dir, m.err, m.filter = dir, nil, ""

	m.all = nil
	if parent := filepath.Dir(dir); parent != dir {
		m.all = append(m.all, entry{label: "..", path: parent, isDir: true, parent: true})
	}
	var dirs, files []entry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			dirs = append(dirs, entry{label: e.Name() + "/", path: path, isDir: true})
		} else if info.Mode().IsRegular() {
			files = append(files, entry{label: e.Name(), path: path, size: info.Size()})
		}
	}
	byName := func(s []entry) {
		sort.Slice(s, func(i, j int) bool { return strings.ToLower(s[i].label) < strings.ToLower(s[j].label) })
	}
	byName(dirs)
	byName(files)
	m.all = append(append(m.all, dirs...), files...)
	m.applyFilter()
}

func (m *Model) applyFilter() {
	m.visible = nil
	filter := strings.ToLower(m.filter)
	for _, e := range m.all {
		if filter == "" || (!e.parent && strings.Contains(strings.ToLower(e.label), filter)) {
			m.visible = append(m.visible, e)
		}
	}
	m.cursor, m.offset = 0, 0
}

// View renders the browser with the given width.
func (m *Model) View(width int) string {
	muted := lipgloss.NewStyle().Foreground(styles.Current.TextMuted)
	selected := lipgloss.NewStyle().Foreground(styles.Current.Primary).Bold(true)

	lines := []string{ansi.Truncate(abbreviateHome(m.dir), width-2, "…")}
	if m.filter != "" {
		lines = append(lines, muted.Render("filter: ")+m.filter)
	} else {
		lines = append(lines, "")
	}
	if len(m.visible) == 0 {
		lines = append(lines, muted.Render("  no matching files"))
	}
	for i := m.offset; i < min(len(m.visible), m.offset+m.height); i++ {
		e := m.visible[i]
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "> "
			style = selected
		}
		info := ""
		if !e.isDir {
			info = humanSize(e.size)
		}
		label := ansi.Truncate(e.label, max(width-14, 10), "…")
		padding := max(width-2-lipgloss.Width(label)-lipgloss.Width(info), 1)
		lines = append(lines, cursor+style.Render(label)+strings.Repeat(" ", padding)+muted.Render(info))
	}
	if len(m.visible) > m.height {
		lines = append(lines, muted.Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.visible))))
	}
	if m.err != nil {
		lines = append(lines, styles.Notifier.Error.UnsetMarginLeft().Render(m.err.Error()))
	}
	return strings.Join(lines, "\n")
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func existingDir(start string) string {
	dir, err := filepath.Abs(expandHome(start))
	if err != nil || start == "" {
		dir, _ = os.Getwd()
	}
	for !isDir(dir) {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dir
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

func abbreviateHome(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
