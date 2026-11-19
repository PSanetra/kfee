// Package decoders_page lists the protobuf decoders of a topic and the transformations saved for them.
package decoders_page

import (
	"fmt"
	"kfee/config"
	"kfee/kontext"
	"kfee/styles"
	"kfee/transformation"
	"kfee/ui"
	"kfee/ui/components/border"
	"kfee/ui/components/statusbar"
	ktable "kfee/ui/components/table"
	"kfee/ui/tabs"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	lg "charm.land/lipgloss/v2"
)

// Model lists the decoder rules of a topic. Selecting a rule shows the transformations saved for it,
// the latest one is the one applied. A transformation can be activated again, which moves it to the latest position.
type Model struct {
	topic       string
	cluster     string
	rules       []config.ProtobufRule
	history     map[string][]transformation.Entry
	store       transformation.Store
	transformer *transformation.Transformer
	table       table.Model
	detail      bool
	// entry is the position in the history of the selected rule, oldest first
	entry     int
	navigator tabs.TopicsTabNavigator
}

// New lists the decoders of the topic in the active cluster of cfg. The transformer, when not nil,
// is invalidated after a transformation was activated.
func New(topic string, cfg *config.Config, transformer *transformation.Transformer, navigator tabs.TopicsTabNavigator) *Model {
	m := &Model{
		topic:       topic,
		history:     map[string][]transformation.Entry{},
		store:       transformation.Store{Dir: config.TransformationsDir()},
		transformer: transformer,
		table:       ktable.NewDefaultTable(),
		navigator:   navigator,
	}
	if cluster := cfg.ActiveCluster(); cluster != nil {
		m.cluster = cluster.Name
		if t := cluster.Protobuf.Topic(topic); t != nil {
			m.rules = t.Rules
		}
		for _, rule := range m.rules {
			// unreadable histories are shown as having none, the record view reports the error
			entries, _ := m.store.Load(cluster.Name, topic, rule.ID)
			m.history[rule.ID] = entries
		}
	}
	return m
}

func (m *Model) View(ktx *kontext.ProgramKtx, renderer *ui.Renderer) string {
	if m.detail && len(m.rules) > 0 {
		b := border.New(border.WithTitleFn(func() string {
			return border.Title("Transformations · "+describeRule(m.selected()), false)
		}))
		return renderer.Render(b.View(lg.NewStyle().Width(ktx.WindowWidth - 4).Render(m.detailView())))
	}

	b := border.New(border.WithTitleFn(func() string {
		return border.Title("Decoders of "+m.topic, false)
	}))
	if len(m.rules) == 0 {
		msg := "No protobuf decoder configured for this topic. Add one from a record with p."
		return renderer.Render(b.View(lg.NewStyle().Foreground(styles.Current.TextMuted).Render(msg)))
	}

	width := ktx.WindowWidth - 4
	m.table.SetWidth(ktx.WindowWidth - 2)
	m.table.SetColumns([]table.Column{
		{Title: "#", Width: 3},
		{Title: "Match", Width: width * 35 / 100},
		{Title: "Decodes as", Width: width * 30 / 100},
		{Title: "Latest transformation", Width: max(width-3-width*35/100-width*30/100, 10)},
	})
	var rows []table.Row
	for i, rule := range m.rules {
		rows = append(rows, table.Row{
			fmt.Sprint(i + 1),
			matchOf(rule),
			decodedAs(rule),
			m.latest(rule),
		})
	}
	m.table.SetRows(rows)
	m.table.SetHeight(ktx.AvailableHeight - 2)
	return renderer.Render(b.View(ktable.View(&m.table)))
}

func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.detail {
			return m.updateDetail(key)
		}
		switch key.String() {
		case "esc":
			return m.navigator.ToCachedConsumptionPage()
		case "enter":
			if len(m.rules) > 0 {
				m.detail = true
				m.entry = max(len(m.history[m.selected().ID])-1, 0)
			}
			return nil
		}
	}
	t, cmd := m.table.Update(msg)
	m.table = t
	return cmd
}

// updateDetail moves the entry cursor, activates the entry under it or goes back to the decoders.
func (m *Model) updateDetail(key tea.KeyPressMsg) tea.Cmd {
	history := m.history[m.selected().ID]
	switch key.String() {
	case "esc":
		m.detail = false
	case "up":
		// the newest entry is drawn at the top, so up goes to the next newer entry
		m.entry = min(m.entry+1, len(history)-1)
	case "down":
		m.entry = max(m.entry-1, 0)
	case "a", "enter":
		m.activate()
	}
	return nil
}

// activate removes the selected entry from its position and appends it again with the current time.
func (m *Model) activate() {
	rule := m.selected()
	if len(m.history[rule.ID]) == 0 {
		return
	}
	history, err := m.store.Activate(m.cluster, m.topic, rule.ID, m.entry)
	if err != nil {
		return
	}
	m.history[rule.ID] = history
	m.entry = len(history) - 1
	if m.transformer != nil {
		m.transformer.Invalidate(m.topic)
	}
}

func (m *Model) Shortcuts() []statusbar.Shortcut {
	if m.detail {
		return []statusbar.Shortcut{
			{Name: "Move", Keybinding: "↑/↓"},
			{Name: "Activate", Keybinding: "a/enter"},
			{Name: "Back", Keybinding: "esc"},
		}
	}
	return []statusbar.Shortcut{
		{Name: "Show Transformations", Keybinding: "enter"},
		{Name: "Go Back", Keybinding: "esc"},
	}
}

func (m *Model) Title() string {
	return "Topics / " + m.topic + " / Decoders"
}

// selected is the rule under the cursor.
func (m *Model) selected() config.ProtobufRule {
	i := min(max(m.table.Cursor(), 0), len(m.rules)-1)
	return m.rules[i]
}

// latest is the filter applied last to the rule, or a hint when there is none.
func (m *Model) latest(rule config.ProtobufRule) string {
	history := m.history[rule.ID]
	if len(history) == 0 {
		return "none"
	}
	last := history[len(history)-1]
	filter := strings.Join(strings.Fields(last.Filter), " ")
	return last.Applied.Local().Format("2006-01-02 15:04") + "  " + filter
}

// detailView lists the transformations of the selected rule, the latest one first. The entry
// under the cursor is marked with >, the latest one with ▶.
func (m *Model) detailView() string {
	rule := m.selected()
	history := m.history[rule.ID]
	lines := []string{styles.Help.Footer.Render("id " + rule.ID), ""}
	if len(history) == 0 {
		lines = append(lines, "No transformation saved for this decoder. Press e in a record to add one.")
	}
	for j := len(history) - 1; j >= 0; j-- {
		cursor := "  "
		if j == m.entry {
			cursor = "> "
		}
		marker := " "
		if j == len(history)-1 {
			marker = "▶"
		}
		lines = append(lines, fmt.Sprintf("%s%s %s  %s", cursor, marker, history[j].Applied.Local().Format("2006-01-02 15:04"), history[j].Source))
		for _, filterLine := range strings.Split(history[j].Filter, "\n") {
			lines = append(lines, "       "+filterLine)
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// matchOf tells which records the rule decodes.
func matchOf(rule config.ProtobufRule) string {
	if rule.IsFallback() {
		return "all records"
	}
	return fmt.Sprintf("header %s matches %s", rule.Header, rule.ValueRegex)
}

// decodedAs tells how the rule decodes the records.
func decodedAs(rule config.ProtobufRule) string {
	if rule.Raw {
		return "without schema"
	}
	return rule.MessageType
}

// describeRule tells which records the rule decodes and how.
func describeRule(rule config.ProtobufRule) string {
	return matchOf(rule) + " → " + decodedAs(rule)
}
