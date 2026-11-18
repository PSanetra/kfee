// Package dirpicker lets the user browse to and choose a directory.
package dirpicker

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"kfee/styles"
)

const (
	defaultHeight = 12
	// countBudget limits the directory entries visited to count the files of a directory,
	// so browsing large trees, like the home directory, stays fast.
	countBudget = 2000
)

type kind int

const (
	recentEntry kind = iota
	parentEntry
	dirEntry
)

type entry struct {
	kind  kind
	label string
	path  string
}

type count struct {
	n         int
	truncated bool
}

// Model browses directories, showing how many files with the given extension each contains.
type Model struct {
	dir     string
	ext     string
	recent  []string
	height  int
	all     []entry
	visible []entry
	cursor  int
	offset  int
	filter  string
	err     error
	counts  map[string]count
}

// New starts browsing at start, or its closest existing parent. recent directories are offered first.
func New(start, ext string, recent []string) *Model {
	m := &Model{ext: ext, height: defaultHeight, counts: map[string]count{}}
	for _, r := range recent {
		if abs, err := filepath.Abs(expandHome(r)); err == nil && isDir(abs) && !contains(m.recent, abs) {
			m.recent = append(m.recent, abs)
		}
	}
	m.chdir(existingDir(start))
	return m
}

// Dir is the directory currently browsed.
func (m *Model) Dir() string {
	return m.dir
}

// Filtering reports whether the user typed a filter.
func (m *Model) Filtering() bool {
	return m.filter != ""
}

func (m *Model) ClearFilter() {
	m.filter = ""
	m.applyFilter()
}

// Update handles a key press and reports whether the user chose the current directory.
func (m *Model) Update(msg tea.KeyPressMsg) (chosen bool) {
	switch msg.String() {
	case "ctrl+s":
		return true
	case "up", "ctrl+p":
		m.move(-1)
	case "down", "ctrl+n":
		m.move(1)
	case "pgup":
		m.move(-m.height)
	case "pgdown":
		m.move(m.height)
	case "right", "enter", "tab":
		if m.cursor < len(m.visible) {
			m.chdir(m.visible[m.cursor].path)
		}
	case "left":
		m.chdir(filepath.Dir(m.dir))
	case "backspace":
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
	return false
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
	previous := m.dir
	m.dir, m.err, m.filter = dir, nil, ""

	m.all = nil
	for _, r := range m.recent {
		if r != dir {
			m.all = append(m.all, entry{kind: recentEntry, label: "★ " + abbreviateHome(r), path: r})
		}
	}
	if parent := filepath.Dir(dir); parent != dir {
		m.all = append(m.all, entry{kind: parentEntry, label: "..", path: parent})
	}
	var dirs []entry
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if strings.HasPrefix(e.Name(), ".") || !isDir(path) {
			continue
		}
		dirs = append(dirs, entry{kind: dirEntry, label: e.Name() + "/", path: path})
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].label) < strings.ToLower(dirs[j].label) })
	m.all = append(m.all, dirs...)
	m.applyFilter()

	// coming back from a subdirectory selects it
	for i, e := range m.visible {
		if e.kind == dirEntry && e.path == previous {
			m.cursor = 0
			m.offset = 0
			m.move(i)
			return
		}
	}
}

func (m *Model) applyFilter() {
	m.visible = nil
	filter := strings.ToLower(m.filter)
	for _, e := range m.all {
		if filter == "" || (e.kind != parentEntry && strings.Contains(strings.ToLower(e.label), filter)) {
			m.visible = append(m.visible, e)
		}
	}
	m.cursor, m.offset = 0, 0
	// start on the first subdirectory instead of recent directories or ..
	if filter == "" {
		for i, e := range m.visible {
			if e.kind == dirEntry {
				m.move(i)
				break
			}
		}
	}
}

func (m *Model) countOf(dir string) count {
	if c, ok := m.counts[dir]; ok {
		return c
	}
	var c count
	visited := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		visited++
		if visited > countBudget {
			c.truncated = true
			return filepath.SkipAll
		}
		if d.IsDir() {
			if path != dir && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == m.ext {
			c.n++
		}
		return nil
	})
	m.counts[dir] = c
	return c
}

func (m *Model) countLabel(dir string) string {
	c := m.countOf(dir)
	label := fmt.Sprintf("%d %s", c.n, m.ext)
	if c.truncated {
		label = fmt.Sprintf("%d+ %s", c.n, m.ext)
	}
	return label
}

// View renders the browser with the given width.
func (m *Model) View(width int) string {
	muted := lipgloss.NewStyle().Foreground(styles.Current.TextMuted)
	selected := lipgloss.NewStyle().Foreground(styles.Current.Primary).Bold(true)

	lines := []string{
		ansi.Truncate(abbreviateHome(m.dir), width-12, "…") + muted.Render("  ("+m.countLabel(m.dir)+")"),
	}
	if m.filter != "" {
		lines = append(lines, muted.Render("filter: ")+m.filter)
	} else {
		lines = append(lines, "")
	}

	if len(m.visible) == 0 {
		lines = append(lines, muted.Render("  no matching directories"))
	}
	countWidth := 10
	for i := m.offset; i < min(len(m.visible), m.offset+m.height); i++ {
		e := m.visible[i]
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "> "
			style = selected
		}
		label := ansi.Truncate(e.label, max(width-countWidth-4, 10), "…")
		var info string
		if e.kind != parentEntry {
			info = m.countLabel(e.path)
		}
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

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func contains(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

func abbreviateHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "~" + string(filepath.Separator) + rel
	}
	return path
}
