package record_details_page

import (
	"fmt"
	"kfee/config"
	"kfee/kadmin"
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
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type focus bool
type state bool

const (
	mainViewFocus    focus = true
	headersViewFocus focus = false
	recordView       state = true
	schemaView       state = false
)

type Model struct {
	notifierCmdbar *cmdbar.NotifierCmdBar
	record         *kadmin.ConsumerRecord
	records        []kadmin.ConsumerRecord
	recordIndex    int
	recordVp       *viewport.Model
	headerValueVp  *viewport.Model
	topicName      string
	headerKeyTable *table.Model
	headerRows     []table.Row
	focus          focus
	state          state
	payload        string
	// transformed is the payload with the latest transformation applied, empty if there is none
	transformed string
	// showOriginal shows the payload without the transformation
	showOriginal bool
	// transformEditor is open while the transformation of the rule is edited
	transformEditor *transformEditor
	transformer     *transformation.Transformer
	err             error
	metaInfo        string
	clipWriter      clipper.Writer
	config          *config.Config
	schemaVp        *viewport.Model
	border          *border.Model
	navigator       tabs.TopicsTabNavigator
	protoDecoders   *serdes.ProtobufDecoders
	// compile compiles .proto files, replaceable for tests
	compile    serdes.ProtoCompiler
	protoModal *protoDecoderModal
}

type PayloadCopiedMsg struct {
}

type SchemaCopiedMsg struct {
}

type HeaderValueCopiedMsg struct {
}

type CopyErrorMsg struct {
	Err error
}

type NavigateToNextRecordMsg struct {
}

type NavigateToPrevRecordMsg struct {
}

func (m *Model) View(ktx *kontext.ProgramKtx, renderer *ui.Renderer) string {

	notifierCmdbarView := m.notifierCmdbar.View(ktx, renderer)

	width := int(float64(ktx.WindowWidth) * 0.70)
	height := ktx.AvailableHeight - 2

	mainView := m.mainView(width, height)
	sidebarView := m.sidebarView(ktx, width, height)

	content := ui.JoinVertical(
		lipgloss.Top,
		notifierCmdbarView,
		lipgloss.NewStyle().Render(lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.border.View(mainView),
			sidebarView)))
	if m.protoModal != nil {
		content = ui.Overlay(content, m.protoModal.View(ktx.WindowWidth), ktx.WindowWidth, ktx.AvailableHeight)
	}
	if m.transformEditor != nil {
		content = ui.Overlay(content, m.transformEditor.View(ktx.WindowWidth, ktx.AvailableHeight), ktx.WindowWidth, ktx.AvailableHeight)
	}
	return content
}

func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if m.recordVp == nil && m.err == nil {
		return nil
	}

	var cmds []tea.Cmd

	_, _, cmd := m.notifierCmdbar.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case llmGeneratedMsg:
		m.handleGenerated(msg)
		return nil
	case protoDecoderClosedMsg:
		m.protoModal = nil
		return tea.Batch(cmds...)
	case protoDecoderSavedMsg:
		m.protoModal = nil
		if cluster := m.config.ActiveCluster(); cluster != nil {
			m.config.SetProtobufTopic(cluster.Name, msg.topic)
		}
		m.protoDecoders.SetTopic(msg.topic, msg.schema)
		m.redecode()
		m.notifierCmdbar.Notifier.ShowSuccessMsg("Protobuf decoder saved")
		return tea.Batch(append(cmds, m.notifierCmdbar.Notifier.AutoHideCmd("record-details-page"))...)
	case protoDecoderRemovedMsg:
		m.protoModal = nil
		if cluster := m.config.ActiveCluster(); cluster != nil {
			m.config.DeleteProtobufTopic(cluster.Name, msg.topic)
		}
		m.protoDecoders.DeleteTopic(msg.topic)
		m.redecode()
		m.notifierCmdbar.Notifier.ShowSuccessMsg("Protobuf decoder removed")
		return tea.Batch(append(cmds, m.notifierCmdbar.Notifier.AutoHideCmd("record-details-page"))...)
	}

	// the transformation editor captures all key presses while open
	if m.transformEditor != nil {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			return m.updateTransformEditor(key)
		}
	}

	// the protobuf decoder modal captures all key presses while open
	if m.protoModal != nil {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			return m.protoModal.Update(msg)
		}
		cmds = append(cmds, m.protoModal.Update(msg))
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m.navigator.ToCachedConsumptionPage()
		case "p":
			m.openProtoModal()
		case "ctrl+n":
			cmds = m.handleNavigateToNext(cmds)
		case "ctrl+p":
			cmds = m.handleNavigateToPrev(cmds)
		case "h", "left", "right":
			if len(m.record.Headers) >= 1 {
				m.focus = !m.focus
				m.border.Focused = m.focus == mainViewFocus
			}
		case "e":
			m.openTransformEditor()
		case "t":
			m.toggleOriginal()
		case "c":
			cmds = m.handleCopy(cmds)
		case "tab":
			if m.record.Payload.Schema != "" && m.focus == mainViewFocus {
				m.state = !m.state
				m.border.NextTab()
			}
		default:
			cmds = m.updatedFocussedArea(msg, cmds)
		}
	case NavigateToNextRecordMsg:
		cmds = m.loadRecordAtIndex(m.recordIndex+1, cmds)
	case NavigateToPrevRecordMsg:
		cmds = m.loadRecordAtIndex(m.recordIndex-1, cmds)
	}

	return tea.Batch(cmds...)
}

func (m *Model) mainView(width int, height int) string {
	var mainView string
	if m.state == recordView {
		mainView = m.recordView(width, height)
	} else {
		mainView = m.schemaView(width, height)
	}

	return mainView
}

func (m *Model) schemaView(width int, height int) string {
	if m.schemaVp == nil {
		schemaVp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
		m.schemaVp = &schemaVp
		if m.err == nil {
			m.schemaVp.SetContent(lipgloss.NewStyle().
				Padding(0, 1).
				Render(ui.PrettyPrintJson(m.record.Payload.Schema)))
		}
	} else {
		m.schemaVp.SetHeight(height)
		m.schemaVp.SetWidth(width)
	}
	return m.schemaVp.View()
}

func (m *Model) sidebarView(ktx *kontext.ProgramKtx, payloadWidth int, height int) string {
	headersTableStyle := m.headerStyle()
	sideBarWidth := ktx.WindowWidth - (payloadWidth + 7)

	var headerSideBar string
	if len(m.record.Headers) == 0 {
		headerSideBar = ui.JoinVertical(
			lipgloss.Top,
			lipgloss.NewStyle().Padding(1).Render(m.metaInfo),
			lipgloss.JoinVertical(lipgloss.Center, lipgloss.NewStyle().Padding(1).Render("No headers present")),
		)
	} else {
		headerValueTableHeight := len(m.record.Headers) + 4

		headerValueVp := viewport.New(viewport.WithWidth(sideBarWidth), viewport.WithHeight(height-headerValueTableHeight-4))
		m.headerValueVp = &headerValueVp
		m.headerKeyTable.SetColumns([]table.Column{
			{"Header Key", sideBarWidth},
		})
		m.headerKeyTable.SetHeight(headerValueTableHeight)
		m.headerKeyTable.SetWidth(ktable.RowWidth(m.headerKeyTable.Columns()))
		m.headerKeyTable.SetRows(m.headerRows)

		headerValueLine := strings.Builder{}
		for i := 0; i < sideBarWidth; i++ {
			headerValueLine.WriteString("─")
		}

		headerValue := m.selectedHeaderValue()
		// wrap long values, breaking words longer than the sidebar like ids or tokens
		m.headerValueVp.SetContent("Header Value\n" + headerValueLine.String() + "\n" + ansi.Wrap(headerValue, max(sideBarWidth, 1), ""))

		headerSideBar = ui.JoinVertical(
			lipgloss.Top,
			lipgloss.NewStyle().Padding(1).Render(m.metaInfo),
			headersTableStyle.Render(lipgloss.JoinVertical(lipgloss.Top, ktable.View(m.headerKeyTable), m.headerValueVp.View())),
		)
	}
	return headerSideBar
}

func (m *Model) selectedHeaderValue() string {
	selectedRow := m.headerKeyTable.SelectedRow()
	if selectedRow == nil {
		if len(m.record.Headers) > 0 {
			return m.record.Headers[0].Value.String()
		}
	} else {
		return m.record.Headers[m.headerKeyTable.Cursor()].Value.String()
	}
	return ""
}

func (m *Model) recordView(payloadWidth int, height int) string {
	if m.recordVp == nil {
		recordVp := viewport.New(viewport.WithWidth(payloadWidth), viewport.WithHeight(height))
		m.recordVp = &recordVp
		if m.err == nil {
			m.recordVp.SetContent(lipgloss.NewStyle().
				Padding(0, 1).
				Render(m.shownPayload()))
		} else {
			m.recordVp.SetContent(lipgloss.NewStyle().
				AlignHorizontal(lipgloss.Center).
				AlignVertical(lipgloss.Center).
				Width(payloadWidth).
				Height(height).
				Render(lipgloss.NewStyle().
					Bold(true).
					Padding(1).
					Foreground(styles.Current.TextMuted).
					Render("Unable to render payload")))
		}
	} else {
		m.recordVp.SetHeight(height)
		m.recordVp.SetWidth(payloadWidth)
	}
	return m.recordVp.View()
}

func (m *Model) headerStyle() lipgloss.Style {
	var headersTableStyle lipgloss.Style
	if m.focus == mainViewFocus {
		headersTableStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			Padding(0).
			Margin(0).
			BorderForeground(styles.Current.BorderBlur)
	} else {
		headersTableStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			Padding(0).
			Margin(0).
			BorderForeground(styles.Current.BorderFocus)
	}
	return headersTableStyle
}

func (m *Model) handleCopy(cmds []tea.Cmd) []tea.Cmd {
	if m.focus == mainViewFocus {
		var copiedValue string
		if m.state == schemaView {
			copiedValue = ansi.Strip(m.record.Payload.Schema)
		} else {
			copiedValue = ansi.Strip(m.shownPayload())
		}

		err := m.clipWriter.Write(copiedValue)

		if err != nil {
			cmds = append(cmds, ui.PublishMsg(CopyErrorMsg{Err: err}))
		} else if m.state == recordView {
			cmds = append(cmds, ui.PublishMsg(PayloadCopiedMsg{}))
		} else {
			cmds = append(cmds, ui.PublishMsg(SchemaCopiedMsg{}))
		}
	} else {
		err := m.clipWriter.Write(m.selectedHeaderValue())
		if err != nil {
			cmds = append(cmds, ui.PublishMsg(CopyErrorMsg{Err: err}))
		} else {
			cmds = append(cmds, ui.PublishMsg(HeaderValueCopiedMsg{}))
		}
	}
	return cmds
}

func (m *Model) handleNavigateToNext(cmds []tea.Cmd) []tea.Cmd {
	if m.recordIndex >= len(m.records)-1 {
		m.notifierCmdbar.Notifier.ShowError(fmt.Errorf("no more records"))
		return cmds
	}
	cmds = append(cmds, ui.PublishMsg(NavigateToNextRecordMsg{}))
	return cmds
}

func (m *Model) handleNavigateToPrev(cmds []tea.Cmd) []tea.Cmd {
	if m.recordIndex <= 0 {
		m.notifierCmdbar.Notifier.ShowError(fmt.Errorf("no previous records"))
		return cmds
	}
	cmds = append(cmds, ui.PublishMsg(NavigateToPrevRecordMsg{}))
	return cmds
}

func (m *Model) loadRecordAtIndex(index int, cmds []tea.Cmd) []tea.Cmd {
	if index < 0 || index >= len(m.records) {
		return cmds
	}
	m.record = &m.records[index]
	m.recordIndex = index
	m.resetViews()
	m.rebuildHeaderRows()
	m.updateMetaInfo()
	return cmds
}

func (m *Model) rebuildHeaderRows() {
	sort.SliceStable(m.record.Headers, func(i, j int) bool {
		return m.record.Headers[i].Key < m.record.Headers[j].Key
	})
	m.headerRows = nil
	for _, header := range m.record.Headers {
		m.headerRows = append(m.headerRows, table.Row{header.Key})
	}
}

func (m *Model) resetViews() {
	m.recordVp = nil
	m.schemaVp = nil
}

func (m *Model) updateMetaInfo() {
	m.metaInfo = metaInfo(m.record)
	m.transformed = ""
	if m.record.Err != nil {
		m.err = m.record.Err
		m.notifierCmdbar.Notifier.ShowError(m.record.Err)
	} else {
		m.err = nil
		m.payload = ui.PrettyPrintJson(m.record.Payload.Value)
		m.refreshTransformation()
	}
}

func metaInfo(record *kadmin.ConsumerRecord) string {
	key := record.Key
	if key == "" {
		key = "<null>"
	}
	info := fmt.Sprintf("key: %s\ntimestamp: %s", key, record.Timestamp.Format(time.UnixDate))
	switch record.Payload.MessageType {
	case "":
	case serdes.RawMessageType:
		info += "\ntype: protobuf without schema\n(keys are field numbers)"
	default:
		info += "\ntype: " + record.Payload.MessageType
	}
	return info
}

func (m *Model) openProtoModal() {
	var preselectedHeader string
	if m.focus == headersViewFocus && len(m.record.Headers) > 0 {
		preselectedHeader = m.record.Headers[m.headerKeyTable.Cursor()].Key
	}
	var protobufCfg *config.ProtobufConfig
	if cluster := m.config.ActiveCluster(); cluster != nil {
		protobufCfg = cluster.Protobuf
	}
	m.protoModal = newProtoDecoderModal(
		m.topicName,
		m.record,
		protobufCfg,
		m.protoDecoders.Protoc(),
		m.compile,
		preselectedHeader,
	)
}

// redecode decodes the records again after the protobuf decoder configuration changed.
func (m *Model) redecode() {
	records := make([]kadmin.ConsumerRecord, len(m.records))
	for i, r := range m.records {
		records[i] = r.RedecodeProtobuf(m.protoDecoders, m.topicName)
	}
	m.records = records

	if m.recordIndex >= 0 && m.recordIndex < len(m.records) {
		m.loadRecordAtIndex(m.recordIndex, nil)
		return
	}
	record := m.record.RedecodeProtobuf(m.protoDecoders, m.topicName)
	m.record = &record
	m.resetViews()
	m.updateMetaInfo()
}

func (m *Model) updatedFocussedArea(msg tea.Msg, cmds []tea.Cmd) []tea.Cmd {
	// only update the component if no error is present
	if m.err != nil {
		return cmds
	}

	if m.focus == mainViewFocus {
		if m.state == recordView {
			vp, cmd := m.recordVp.Update(msg)
			cmds = append(cmds, cmd)
			m.recordVp = &vp
		} else {
			vp, cmd := m.schemaVp.Update(msg)
			cmds = append(cmds, cmd)
			m.schemaVp = &vp
		}
	} else {
		t, cmd := m.headerKeyTable.Update(msg)
		cmds = append(cmds, cmd)
		m.headerKeyTable = &t
	}
	return cmds
}

func (m *Model) Shortcuts() []statusbar.Shortcut {
	if m.transformEditor != nil {
		return m.transformEditor.Shortcuts()
	}
	if m.protoModal != nil {
		return m.protoModal.Shortcuts()
	}
	protoShortcut := statusbar.Shortcut{Name: "Protobuf Decoder", Keybinding: "p"}

	whatToCopy := "Header Value"
	if m.focus == mainViewFocus {
		if m.state == schemaView {
			whatToCopy = "Schema"
		} else {
			whatToCopy = "Record"
		}
	}
	if m.err == nil {
		shortcuts := []statusbar.Shortcut{
			{"Toggle Headers/Content", "h/left/right"},
			{"Go Back", "esc"},
			{"Copy " + whatToCopy, "c"},
			protoShortcut,
		}

		if m.record.Payload.RuleID != "" {
			shortcuts = append(shortcuts, statusbar.Shortcut{Name: "Transform", Keybinding: "e"})
		}
		if m.transformed != "" {
			shortcuts = append(shortcuts, statusbar.Shortcut{Name: m.transformToggleName(), Keybinding: "t"})
		}

		if len(m.records) > 1 {
			shortcuts = append(shortcuts, []statusbar.Shortcut{
				{"Next Record", "ctrl+n"},
				{"Prev Record", "ctrl+p"},
			}...)
		}

		if m.config.ActiveCluster().HasSchemaRegistry() && m.focus == mainViewFocus {
			shortcuts = append(shortcuts, statusbar.Shortcut{
				Name:       "Toggle Record/Schema",
				Keybinding: "<tab>",
			})
		}

		return shortcuts
	}

	return []statusbar.Shortcut{
		{"Go Back", "esc"},
		protoShortcut,
	}
}

func (m *Model) Title() string {
	return "Topics / " + m.topicName + " / Partition / " + strconv.FormatInt(m.record.Partition, 10) +
		" / Offset / " + strconv.FormatInt(m.record.Offset, 10)
}

func New(
	record *kadmin.ConsumerRecord,
	topicName string,
	records []kadmin.ConsumerRecord,
	recordIndex int,
	clipWriter clipper.Writer,
	ktx *kontext.ProgramKtx,
	navigator tabs.TopicsTabNavigator,
) *Model {
	headersTable := ktable.NewDefaultTable()

	var headerRows []table.Row
	sort.SliceStable(record.Headers, func(i, j int) bool {
		return record.Headers[i].Key < record.Headers[j].Key
	})
	for _, header := range record.Headers {
		headerRows = append(headerRows, table.Row{header.Key})
	}

	notifierCmdBar := cmdbar.NewNotifierCmdBar("record-details-page")

	var (
		payload string
		err     error
	)
	if record.Err == nil {
		payload = ui.PrettyPrintJson(record.Payload.Value)
	} else {
		err = record.Err
		notifierCmdBar.Notifier.ShowError(record.Err)
	}

	cmdbar.BindNotificationHandler(notifierCmdBar, func(msg PayloadCopiedMsg, m *notifier.Model) (bool, tea.Cmd) {
		m.ShowSuccessMsg("Payload copied")
		return true, m.AutoHideCmd("record-details-page")
	})
	cmdbar.BindNotificationHandler(notifierCmdBar, func(msg SchemaCopiedMsg, m *notifier.Model) (bool, tea.Cmd) {
		m.ShowSuccessMsg("Schema copied")
		return true, m.AutoHideCmd("record-details-page")
	})
	cmdbar.BindNotificationHandler(notifierCmdBar, func(msg HeaderValueCopiedMsg, m *notifier.Model) (bool, tea.Cmd) {
		m.ShowSuccessMsg("Header Value copied")
		return true, m.AutoHideCmd("record-details-page")
	})
	cmdbar.BindNotificationHandler(notifierCmdBar, func(msg CopyErrorMsg, m *notifier.Model) (bool, tea.Cmd) {
		m.ShowErrorMsg("Copy failed", msg.Err)
		return true, m.AutoHideCmd("record-details-page")
	})

	var tabs []border.Tab
	if record.Payload.Schema != "" {
		tabs = []border.Tab{
			{Title: "Record", TabLabel: "record"},
			{Title: "Schema", TabLabel: "record"},
		}
	}

	m := &Model{
		record:         record,
		records:        records,
		recordIndex:    recordIndex,
		topicName:      topicName,
		headerKeyTable: &headersTable,
		focus:          mainViewFocus,
		headerRows:     headerRows,
		payload:        payload,
		err:            err,
		metaInfo:       metaInfo(record),
		clipWriter:     clipWriter,
		notifierCmdbar: notifierCmdBar,
		config:         ktx.Config(),
		state:          recordView,
		navigator:      navigator,
		protoDecoders:  ktx.ProtoDecoders(),
		transformer:    ktx.Transformer(),
		compile:        serdes.CompileProtoDir,
	}
	m.refreshTransformation()
	// the payload type changes when navigating records or changing the protobuf decoder
	m.border = border.New(
		border.WithTabs(tabs...),
		border.WithTitleFn(func() string { return "[ " + m.record.PayloadType() + " ]" + m.transformationMarker() }))
	return m
}
