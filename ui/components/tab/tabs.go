package tab

import (
	tea "charm.land/bubbletea/v2"
	lg "charm.land/lipgloss/v2"
	"kfee/keymap"
	"kfee/kontext"
	"kfee/styles"
	"kfee/ui"
	"strings"
)

var (
	// ActionHelp opens and closes the help. The footer shows its key when shortcuts are left out.
	ActionHelp = keymap.Define("app.help", "global", "f1")
)

type Label string

type Tab struct {
	Title string
	Label
}

type Model struct {
	tabs []Tab
	// zero indexed
	activeTab int
}

func (m *Model) View(ctx *kontext.ProgramKtx, renderer *ui.Renderer) string {
	if len(m.tabs) == 0 {
		return ""
	}
	tabsToRender := make([]string, len(m.tabs))
	for i, t := range m.tabs {
		var tab string
		if i == m.activeTab {
			tab = styles.Tab.Active.Render(t.Title)
		} else {
			tab = styles.Tab.Tab.Render(t.Title)
		}
		tabsToRender = append(tabsToRender, tab)
	}
	renderedTabs := lg.JoinHorizontal(lg.Top, tabsToRender...)
	tabLine := strings.Builder{}
	leftOverSpace := ctx.WindowWidth - lg.Width(renderedTabs)
	for i := 0; i < leftOverSpace; i++ {
		tabLine.WriteString("─")
	}
	s := renderedTabs + tabLine.String()
	return renderer.Render(s)
}

func (m *Model) Update(msg tea.Msg) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if keymap.Matches(msg, keymap.TabsPrevious) {
			m.Prev()
		} else if keymap.Matches(msg, keymap.TabsNext) {
			m.Next()
		}
	}
}

func (m *Model) Next() {
	if m.activeTab < m.numberOfTabs()-1 {
		m.activeTab++
	}
}

func (m *Model) Prev() {
	if m.activeTab > 0 {
		m.activeTab--
	}
}

func (m *Model) numberOfTabs() int {
	return len(m.tabs)
}

func (m *Model) GoToTab(label Label) {
	for i, t := range m.tabs {
		if t.Label == label {
			m.activeTab = i
		}
	}
}

func (m *Model) ActiveTab() Tab {
	if m.tabs == nil {
		return Tab{}
	}
	return m.tabs[m.activeTab]
}

func New(tabs ...Tab) Model {
	return Model{
		tabs: tabs,
	}
}
