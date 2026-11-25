// Package decoders_page lists the decoders of a topic and the transformations saved for them.
package decoders_page

import (
	"fmt"
	"kfee/config"
	"kfee/keymap"
	"kfee/kontext"
	"kfee/serdes"
	"kfee/styles"
	"kfee/transformation"
	"kfee/ui"
	"kfee/ui/clipper"
	"kfee/ui/components/border"
	"kfee/ui/components/cmdbar"
	"kfee/ui/components/notifier"
	"kfee/ui/components/statusbar"
	ktable "kfee/ui/components/table"
	"kfee/ui/tabs"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lg "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// SchemaCopiedMsg is sent when the schema of a decoder was copied.
type SchemaCopiedMsg struct{}

// CopyErrorMsg is sent when copying failed.
type CopyErrorMsg struct {
	Err error
}

// Actions of the decoders list, and of the schema and transformations shown for a decoder.
var (
	decodersShow        = keymap.Define("decoders.show_transformations", "decoders", "enter")
	decodersCopy        = keymap.Define("decoders.copy_schema", "decoders", "c")
	decodersBack        = keymap.Define("decoders.back", "decoders", "esc")
	decodersToggleFocus = keymap.Define("decoders.toggle_focus", "decoders", "left", "right")
	decodersActivate    = keymap.Define("decoders.activate", "decoders", "a", "enter")
	decodersCloseDetail = keymap.Define("decoders.close_detail", "decoders", "esc")
)

// avroPrefix marks the decoder ID of an Avro subject, see serdes and kadmin.
const avroPrefix = "avro:"

// decoder is a protobuf rule of the topic or an Avro subject seen on its records.
type decoder struct {
	// id is the key of the transformations of the decoder
	id      string
	match   string
	decodes string
	// schema is the schema the decoder decodes with, shown next to the transformations
	schema string
}

// Model lists the decoders of a topic. Selecting a decoder shows its schema and the transformations
// saved for it, the latest one is the one applied. A transformation can be activated again, which
// moves it to the latest position.
type Model struct {
	topic       string
	cluster     string
	decoders    []decoder
	history     map[string][]transformation.Entry
	store       transformation.Store
	transformer *transformation.Transformer
	table       table.Model
	detail      bool
	// focusSchema is set while the schema is focused, otherwise the transformations are
	focusSchema bool
	// schemaVp shows the schema of the selected decoder, scrollable
	schemaVp *viewport.Model
	// entry is the position in the history of the selected decoder, oldest first
	entry     int
	navigator tabs.TopicsTabNavigator
	// clipWriter copies the schema of the selected decoder
	clipWriter     clipper.Writer
	notifierCmdbar *cmdbar.NotifierCmdBar
}

// New lists the decoders of the topic in the active cluster of cfg: its protobuf rules, described
// with protoDecoders, and the Avro decoders seen on its records, given by their IDs with the schema
// of their records. The transformer, when not nil, is invalidated after a transformation was activated.
func New(
	topic string,
	cfg *config.Config,
	transformer *transformation.Transformer,
	protoDecoders *serdes.ProtobufDecoders,
	navigator tabs.TopicsTabNavigator,
	avroSchemas map[string]string,
	clipWriter clipper.Writer,
) *Model {
	notifierCmdBar := cmdbar.NewNotifierCmdBar("decoders-page")
	cmdbar.BindNotificationHandler(notifierCmdBar, func(msg SchemaCopiedMsg, m *notifier.Model) (bool, tea.Cmd) {
		m.ShowSuccessMsg("Schema copied")
		return true, m.AutoHideCmd("decoders-page")
	})
	cmdbar.BindNotificationHandler(notifierCmdBar, func(msg CopyErrorMsg, m *notifier.Model) (bool, tea.Cmd) {
		m.ShowErrorMsg("Copy failed", msg.Err)
		return true, m.AutoHideCmd("decoders-page")
	})
	m := &Model{
		topic:          topic,
		history:        map[string][]transformation.Entry{},
		store:          transformation.Store{Dir: config.TransformationsDir()},
		transformer:    transformer,
		table:          ktable.NewDefaultTable(),
		navigator:      navigator,
		clipWriter:     clipWriter,
		notifierCmdbar: notifierCmdBar,
	}
	if cluster := cfg.ActiveCluster(); cluster != nil {
		m.cluster = cluster.Name
		if t := cluster.Protobuf.Topic(topic); t != nil {
			for _, rule := range t.Rules {
				m.decoders = append(m.decoders, decoder{
					id:      rule.ID,
					match:   matchOf(rule),
					decodes: decodedAs(rule),
					schema:  protoSchema(protoDecoders, topic, rule),
				})
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(avroSchemas)) {
		subject := strings.TrimPrefix(id, avroPrefix)
		m.decoders = append(m.decoders, decoder{
			id:      id,
			match:   "Avro schema of " + subject,
			decodes: "Avro",
			schema:  ui.FormatPayload(avroSchemas[id]),
		})
	}
	for _, d := range m.decoders {
		// unreadable histories are shown as having none, the record view reports the error
		entries, _ := m.store.Load(m.cluster, topic, d.id)
		m.history[d.id] = entries
	}
	return m
}

// View shows the notification above the decoders. The notification takes its height first, so the
// content is laid out in the rest.
func (m *Model) View(ktx *kontext.ProgramKtx, renderer *ui.Renderer) string {
	notifierView := m.notifierCmdbar.View(ktx, renderer)
	return ui.JoinVertical(lg.Top, notifierView, m.body(ktx, renderer))
}

func (m *Model) body(ktx *kontext.ProgramKtx, renderer *ui.Renderer) string {
	if m.detail && len(m.decoders) > 0 {
		return renderer.Render(m.detailView(ktx))
	}

	b := border.New(border.WithTitleFn(func() string {
		return border.Title("Decoders of "+m.topic, false)
	}))
	if len(m.decoders) == 0 {
		msg := "No decoder known for this topic. Protobuf decoders are added from a record with p, Avro decoders show up once records are read."
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
	for i, d := range m.decoders {
		rows = append(rows, table.Row{
			fmt.Sprint(i + 1),
			d.match,
			d.decodes,
			m.latest(d),
		})
	}
	m.table.SetRows(rows)
	m.table.SetHeight(ktx.AvailableHeight - 2)
	return renderer.Render(b.View(ktable.View(&m.table)))
}

// detailView is the selected decoder: the schema on the left, scrollable, the transformations on the
// right. Like the record view, h, left and right switch the focus, the focused pane takes the keys.
func (m *Model) detailView(ktx *kontext.ProgramKtx) string {
	width := ktx.WindowWidth - 4
	leftWidth := width * 45 / 100
	rightWidth := width - leftWidth - 2
	height := max(ktx.AvailableHeight-2, 6)

	m.schemaVp.SetWidth(leftWidth - 2)
	m.schemaVp.SetHeight(height - 2)
	schemaBorder := border.New(border.WithTitleFn(func() string {
		return border.Title("Schema", false)
	}))
	schemaBorder.Focused = m.focusSchema

	transformationsBorder := border.New(border.WithTitleFn(func() string {
		return border.Title("Transformations · "+m.selected().describe(), false)
	}))
	transformationsBorder.Focused = !m.focusSchema

	left := lg.NewStyle().Width(leftWidth).Render(schemaBorder.View(m.schemaVp.View()))
	right := lg.NewStyle().Width(rightWidth).MaxHeight(height).Render(
		transformationsBorder.View(lg.NewStyle().Width(rightWidth - 2).Render(m.detailText())),
	)
	return lg.JoinHorizontal(lg.Top, left, right)
}

func (m *Model) Update(msg tea.Msg) tea.Cmd {
	_, _, notifyCmd := m.notifierCmdbar.Update(msg)
	return tea.Batch(notifyCmd, m.update(msg))
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if keymap.Matches(key, decodersCopy) && len(m.decoders) > 0 {
			return m.copySchema()
		}
		if m.detail {
			return m.updateDetail(key)
		}
		switch {
		case keymap.Matches(key, decodersBack):
			return m.navigator.ToCachedConsumptionPage()
		case keymap.Matches(key, decodersShow):
			if len(m.decoders) > 0 {
				m.openDetail()
			}
			return nil
		}
	}
	t, cmd := m.table.Update(msg)
	m.table = t
	return cmd
}

// openDetail shows the selected decoder, its schema is focused and starts at the top.
func (m *Model) openDetail() {
	m.detail = true
	m.focusSchema = true
	m.entry = max(len(m.history[m.selected().id])-1, 0)
	vp := viewport.New(viewport.WithWidth(1), viewport.WithHeight(1))
	vp.SetContent(m.selected().schema)
	m.schemaVp = &vp
}

// updateDetail toggles the focus, scrolls the schema or moves through the transformations, and
// activates the transformation under the cursor. esc goes back to the decoders.
func (m *Model) updateDetail(key tea.KeyPressMsg) tea.Cmd {
	switch {
	case keymap.Matches(key, decodersCloseDetail):
		m.detail = false
		return nil
	case keymap.Matches(key, decodersToggleFocus):
		m.focusSchema = !m.focusSchema
		return nil
	}

	if m.focusSchema {
		vp, cmd := m.schemaVp.Update(key)
		m.schemaVp = &vp
		return cmd
	}

	history := m.history[m.selected().id]
	switch {
	case keymap.Matches(key, keymap.TableUp):
		// the newest entry is drawn at the top, so up goes to the next newer entry
		m.entry = min(m.entry+1, len(history)-1)
	case keymap.Matches(key, keymap.TableDown):
		m.entry = max(m.entry-1, 0)
	case keymap.Matches(key, decodersActivate):
		m.activate()
	}
	return nil
}

// activate removes the selected entry from its position and appends it again with the current time.
func (m *Model) activate() {
	d := m.selected()
	if len(m.history[d.id]) == 0 {
		return
	}
	history, err := m.store.Activate(m.cluster, m.topic, d.id, m.entry)
	if err != nil {
		return
	}
	m.history[d.id] = history
	m.entry = len(history) - 1
	if m.transformer != nil {
		m.transformer.Invalidate(m.topic)
	}
}

func (m *Model) Shortcuts() []statusbar.Shortcut {
	if m.detail {
		shortcuts := []statusbar.Shortcut{{Name: "Toggle Schema/Transformations", Keybinding: keymap.Hint(decodersToggleFocus)}}
		if m.focusSchema {
			// the schema is scrolled by its viewport, which keeps its own keys
			shortcuts = append(shortcuts, statusbar.Shortcut{Name: "Scroll", Keybinding: "↑/↓"})
		} else {
			shortcuts = append(shortcuts,
				statusbar.Shortcut{Name: "Move", Keybinding: keymap.Hint(keymap.TableUp) + "/" + keymap.Hint(keymap.TableDown)},
				statusbar.Shortcut{Name: "Activate", Keybinding: keymap.Hint(decodersActivate)},
			)
		}
		return append(shortcuts,
			statusbar.Shortcut{Name: "Copy Schema", Keybinding: keymap.Hint(decodersCopy)},
			statusbar.Shortcut{Name: "Back", Keybinding: keymap.Hint(decodersCloseDetail)},
		)
	}
	shortcuts := []statusbar.Shortcut{{Name: "Show Transformations", Keybinding: keymap.Hint(decodersShow)}}
	if len(m.decoders) > 0 {
		shortcuts = append(shortcuts, statusbar.Shortcut{Name: "Copy Schema", Keybinding: keymap.Hint(decodersCopy)})
	}
	return append(shortcuts, statusbar.Shortcut{Name: "Go Back", Keybinding: keymap.Hint(decodersBack)})
}

func (m *Model) Title() string {
	return "Topics / " + m.topic + " / Decoders"
}

// selected is the decoder under the cursor.
func (m *Model) selected() decoder {
	i := min(max(m.table.Cursor(), 0), len(m.decoders)-1)
	return m.decoders[i]
}

// copySchema copies the schema of the decoder under the cursor, as shown.
func (m *Model) copySchema() tea.Cmd {
	if err := m.clipWriter.Write(ansi.Strip(m.selected().schema)); err != nil {
		return ui.PublishMsg(CopyErrorMsg{Err: err})
	}
	return ui.PublishMsg(SchemaCopiedMsg{})
}

// latest is the filter applied last to the decoder, or a hint when there is none.
func (m *Model) latest(d decoder) string {
	history := m.history[d.id]
	if len(history) == 0 {
		return "none"
	}
	last := history[len(history)-1]
	filter := strings.Join(strings.Fields(last.Filter), " ")
	return last.Applied.Local().Format("2006-01-02 15:04") + "  " + filter
}

// detailText lists the transformations of the selected decoder, the latest one first. The entry
// under the cursor is marked with >, the latest one with ▶.
func (m *Model) detailText() string {
	d := m.selected()
	history := m.history[d.id]
	lines := []string{styles.Help.Footer.Render("id " + d.id), ""}
	if len(history) == 0 {
		lines = append(lines, "No transformation saved for this decoder. Press e in a record to add one.")
	}
	for j := len(history) - 1; j >= 0; j-- {
		cursor := "  "
		if j == m.entry && !m.focusSchema {
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

// protoSchema describes the message type of a protobuf rule, or says that the rule decodes without schema.
func protoSchema(protoDecoders *serdes.ProtobufDecoders, topic string, rule config.ProtobufRule) string {
	if rule.Raw {
		return "Protobuf without schema: the keys of the JSON are field numbers."
	}
	described, err := protoDecoders.Describe(topic, rule.MessageType)
	if err != nil {
		return "Unable to describe the schema: " + err.Error()
	}
	return described
}

// matchOf tells which records the protobuf rule decodes.
func matchOf(rule config.ProtobufRule) string {
	if rule.IsFallback() {
		return "all records"
	}
	return fmt.Sprintf("header %s matches %s", rule.Header, rule.ValueRegex)
}

// decodedAs tells how the protobuf rule decodes the records.
func decodedAs(rule config.ProtobufRule) string {
	if rule.Raw {
		return "without schema"
	}
	return rule.MessageType
}

// describe tells which records the decoder decodes and how.
func (d decoder) describe() string {
	return d.match + " → " + d.decodes
}
