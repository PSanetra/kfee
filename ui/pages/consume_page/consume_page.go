package consume_page

import (
	"context"
	"fmt"
	"kfee/kadmin"
	"kfee/kontext"
	"kfee/serdes"
	"kfee/styles"
	"kfee/transformation"
	"kfee/ui"
	"kfee/ui/clipper"
	"kfee/ui/components/border"
	"kfee/ui/components/cmdbar"
	"kfee/ui/components/statusbar"
	ktable "kfee/ui/components/table"
	"kfee/ui/pages"
	"kfee/ui/tabs"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"
)

type Model struct {
	table              *table.Model
	preview            *recordPreview
	clipWriter         clipper.Writer
	previewVisible     bool // whether the last render showed the preview
	border             *border.Model
	cmdBar             *ConsumptionCmdBar
	cancelConsumption  context.CancelFunc
	reader             kadmin.RecordReader
	rows               []table.Row
	records            []kadmin.ConsumerRecord
	readDetails        kadmin.ReadDetails
	consuming          bool
	noRecordsAvailable bool
	noRecordsFound     bool
	topic              *kadmin.ListedTopic
	navigator          tabs.TopicsTabNavigator
	search             *searchModal
	// readStarted is false while the page waits for a search to be submitted
	readStarted bool
	// source identifies the current consumption, see isStale
	source  chan kadmin.ConsumerRecord
	goToTop bool
	// protoDecoders decode protobuf records, decodedVersion is the version of their
	// configuration the records were decoded with
	protoDecoders  *serdes.ProtobufDecoders
	decodedVersion uint64
	// progress of searching the topic, nil when not searching
	progress *kadmin.SearchProgress
	// selected is the record selected by the user, nil until the cursor is moved
	selected *rowKey
}

// isStale reports whether msg belongs to a consumption that has been replaced
// by a newer search.
func (m *Model) isStale(msg tea.Msg) bool {
	var source chan kadmin.ConsumerRecord
	switch msg := msg.(type) {
	case kadmin.ConsumerRecordReceived:
		source = msg.Source()
	case kadmin.ConsumptionEndedMsg:
		source = msg.Source()
	}
	return source != nil && source != m.source
}

func (m *Model) View(ktx *kontext.ProgramKtx, renderer *ui.Renderer) string {
	var views []string
	views = append(views, m.cmdBar.View(ktx, renderer))

	if m.noRecordsAvailable {
		m.previewVisible = false
		views = append(views, styles.CenterText(ktx.WindowWidth, ktx.AvailableHeight).
			Render("👀 Empty topic"))
	} else if m.noRecordsFound {
		m.previewVisible = false
		views = append(views, styles.CenterText(ktx.WindowWidth, ktx.AvailableHeight).
			Render("👀 No records found for the given criteria"))
	} else {
		// preview the selected record next to the table when the window is wide enough
		previewW := previewWidth(ktx.WindowWidth)
		tableW := ktx.WindowWidth - previewW

		// fits "2006-01-02 15:04:05", the key takes the rest, the padding of the five columns is included
		tsCol := max(int(float64(tableW)*0.2), 19)
		PCol := max(int(float64(tableW)*0.12), 9)
		oCol := max(int(float64(tableW)*0.1), 9)
		decCol := max(int(float64(tableW)*0.2), 10)
		keyCol := tableW - tsCol - PCol - oCol - decCol - 12
		m.table.SetColumns([]table.Column{
			{Title: "Decoder", Width: decCol},
			{Title: m.cmdBar.sortByCBar.PrefixSortIcon("Key"), Width: keyCol},
			{Title: m.cmdBar.sortByCBar.PrefixSortIcon("Timestamp"), Width: tsCol},
			{Title: m.cmdBar.sortByCBar.PrefixSortIcon("Partition"), Width: PCol},
			{Title: m.cmdBar.sortByCBar.PrefixSortIcon("Offset"), Width: oCol},
		})
		m.table.SetRows(m.rows)
		// a new search starts at the top; the cursor needs rows to move
		if m.goToTop && len(m.rows) > 0 {
			m.table.GotoTop()
			m.goToTop = false
		}
		// records arriving while reading are sorted in, keep the record selected by the user
		if m.selected != nil {
			for i, row := range m.rows {
				if row[colPartition] == m.selected.partition && row[colOffset] == m.selected.offset {
					m.table.SetCursor(i)
					break
				}
			}
		}
		m.table.SetWidth(tableW - 2)
		m.table.SetHeight(ktx.AvailableTableHeight())

		tableView := m.border.View(ktable.View(m.table))
		m.previewVisible = previewW > 0
		if m.previewVisible {
			tableView = lipgloss.JoinHorizontal(lipgloss.Top,
				tableView,
				m.preview.View(m.selectedRecord(), previewW, lipgloss.Height(tableView)))
		}
		views = append(views, tableView)
	}

	content := ui.JoinVertical(lipgloss.Top, views...)
	if m.search != nil {
		content = ui.Overlay(content, m.search.View(ktx.WindowWidth), ktx.WindowWidth, ktx.AvailableHeight)
	}
	return content
}

func (m *Model) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case searchClosedMsg:
		m.search = nil
		if !m.readStarted {
			return m.navigator.ToTopicsPage(false)
		}
		return nil
	case searchSubmittedMsg:
		m.search = nil
		return m.startReading(msg.details)
	}

	// the preview renders large payloads in the background
	if cmd, handled := m.preview.Update(msg); handled {
		return cmd
	}

	if m.isStale(msg) {
		return nil
	}

	// the search modal captures all key presses while open, other messages
	// still reach the page so a running consumption keeps going
	if m.search != nil {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			return m.search.Update(msg)
		}
		cmds = append(cmds, m.search.Update(msg))
	}

	switch msg := msg.(type) {
	case ui.RegainedFocusMsg:
		m.redecodeIfDecodersChanged()
	case tea.KeyPressMsg:
		if msg.String() == "esc" {
			m.cancelConsumption()
			return m.navigator.ToTopicsPage(false)
		} else if (msg.String() == "/" || msg.String() == "ctrl+f") && !m.cmdBar.IsFocussed() {
			m.search = newSearchModal(m.topic, m.readDetails, msg.String() == "ctrl+f")
			return nil
		} else if msg.String() == "d" && !m.cmdBar.IsFocussed() {
			return m.navigator.ToDecodersPage(tabs.DecodersPageDetails{Topic: m.readDetails.TopicName, AvroSchemas: m.avroSchemas()})
		} else if msg.String() == "t" && !m.cmdBar.IsFocussed() && m.selectedRecord() != nil {
			record := m.selectedRecord()
			m.preview.toggleOriginal(record)
			return m.preview.request(record)
		} else if msg.String() == "c" && m.canCopy() {
			record := m.selectedRecord()
			m.preview.copied(record, m.clipWriter.Write(m.preview.copyable(record)))
			return nil
		} else if msg.String() == "f2" {
			m.cancelConsumption()
			m.consuming = false
			cmds = append(cmds, ui.PublishMsg(kadmin.ConsumptionEndedMsg{}))
		} else if msg.String() == "enter" {
			if !m.cmdBar.IsFocussed() {
				if len(m.records) > 0 {
					selectedRow := m.rows[m.table.Cursor()]
					selectedRecord := m.recordForRow(selectedRow)
					recordIndex := m.recordIndexForRow(selectedRow)
					return m.navigator.ToRecordDetailsPage(
						tabs.RecordDetailsPageDetails{
							Record:    &selectedRecord,
							TopicName: m.readDetails.TopicName,
							Records:   m.records,
							Index:     recordIndex,
						})
				}
			}
		}
	case kadmin.EmptyTopicMsg:
		m.noRecordsAvailable = true
		m.consuming = false
	case kadmin.NoRecordsFound:
		m.noRecordsFound = true
		m.consuming = false
	case *kadmin.ReadingStartedMsg:
		m.source = msg.Source()
		m.consuming = true
		m.progress = msg.Progress
		cmds = append(cmds, msg.AwaitRecord)
	case kadmin.ConsumptionEndedMsg:
		m.consuming = false
	case kadmin.ConsumerRecordReceived:
		m.records = append(m.records, msg.Records...)
		cmds = append(cmds, msg.AwaitNextRecord)
	}

	cmd := m.cmdBar.Update(msg)
	cmds = append(cmds, cmd)

	m.rows = m.createRows()

	// make sure table navigation is off when the cmdbar is focussed
	if !m.cmdBar.IsFocussed() {
		t, cmd := m.table.Update(msg)
		m.table = &t
		cmds = append(cmds, cmd)

		// the user moved the cursor, keep that record selected while records arrive
		if _, ok := msg.(tea.KeyPressMsg); ok {
			if row := m.table.SelectedRow(); row != nil {
				m.selected = &rowKey{partition: row[colPartition], offset: row[colOffset]}
			}
		}
	}

	if m.previewVisible {
		cmds = append(cmds, m.preview.request(m.previewedRecord()))
	}

	return tea.Batch(cmds...)
}

// previewedRecord is the record the next render previews: the one the user selected, or the first one.
func (m *Model) previewedRecord() *kadmin.ConsumerRecord {
	if m.selected != nil {
		for _, row := range m.rows {
			if row[colPartition] == m.selected.partition && row[colOffset] == m.selected.offset {
				record := m.recordForRow(row)
				return &record
			}
		}
	}
	return m.selectedRecord()
}

// rowKey identifies the record of a table row.
type rowKey struct {
	partition, offset string
}

// redecodeIfDecodersChanged decodes the records again when the protobuf decoder
// configuration changed, e.g. in the record details page.
func (m *Model) redecodeIfDecodersChanged() {
	version := m.protoDecoders.Version()
	if version == m.decodedVersion {
		return
	}
	m.decodedVersion = version
	// a new slice, as the records are shared with the record details page
	records := make([]kadmin.ConsumerRecord, len(m.records))
	for i, r := range m.records {
		records[i] = r.RedecodeProtobuf(m.protoDecoders, m.readDetails.TopicName)
	}
	m.records = records
}

// canCopy reports whether the payload of the selected record is shown in the preview, so it can be copied.
func (m *Model) canCopy() bool {
	if !m.previewVisible || m.cmdBar.IsFocussed() || m.cmdBar.IsSorting() {
		return false
	}
	record := m.selectedRecord()
	return record != nil && record.Err == nil
}

// selectedRecord returns the record of the table row under the cursor, nil if there is none.
func (m *Model) selectedRecord() *kadmin.ConsumerRecord {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.rows) {
		return nil
	}
	record := m.recordForRow(m.rows[cursor])
	return &record
}

func (m *Model) recordForRow(row table.Row) kadmin.ConsumerRecord {
	offset, _ := strconv.ParseInt(row[colOffset], 10, 64)
	partition, _ := strconv.ParseInt(row[colPartition], 10, 32)
	for _, rec := range m.records {
		if rec.Partition == partition &&
			rec.Offset == offset {
			return rec
		}
	}
	panic(fmt.Sprintf("Record not found for row: %v", row))
}

func (m *Model) recordIndexForRow(row table.Row) int {
	offset, _ := strconv.ParseInt(row[colOffset], 10, 64)
	partition, _ := strconv.ParseInt(row[colPartition], 10, 32)
	for i, rec := range m.records {
		if rec.Partition == partition &&
			rec.Offset == offset {
			return i
		}
	}
	panic(fmt.Sprintf("Record not found for row: %v", row))
}

// columns of the records table, in the order they are shown
const (
	colDecoder = iota
	colKey
	colTimestamp
	colPartition
	colOffset
)

// decoderName names the decoder of a record: the subject of an Avro schema, the message type of a
// protobuf rule, or the raw decoding. Empty when the value is not decoded. Names with dots are
// shortened to their last part, e.g. acme.orders.v1.OrderCancelled shows OrderCancelled.
func decoderName(data serdes.DesData) string {
	var name string
	switch {
	case strings.HasPrefix(data.RuleID, "avro:"):
		name = strings.TrimPrefix(data.RuleID, "avro:")
	case data.MessageType == serdes.RawMessageType:
		return "without schema"
	default:
		name = data.MessageType
	}
	return name[strings.LastIndex(name, ".")+1:]
}

func (m *Model) createRows() []table.Row {
	var rows []table.Row
	for _, rec := range m.records {

		var key string
		if rec.Key == "" {
			key = "<null>"
		} else {
			key = rec.Key
		}

		rows = append(
			rows,
			table.Row{
				decoderName(rec.Payload),
				key,
				rec.Timestamp.Format("2006-01-02 15:04:05"),
				strconv.FormatInt(rec.Partition, 10),
				strconv.FormatInt(rec.Offset, 10),
			},
		)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		var col int
		switch m.cmdBar.sortByCBar.SortedBy().Label {
		case "Key":
			col = colKey
		case "Timestamp":
			col = colTimestamp
		case "Partition":
			col = colPartition
		case "Offset":
			col = colOffset
		default:
			panic(fmt.Sprintf("unexpected sort label: %s", m.cmdBar.sortByCBar.SortedBy().Label))
		}

		if m.cmdBar.sortByCBar.SortedBy().Direction == cmdbar.Asc {
			return rows[i][col] < rows[j][col]
		}
		return rows[i][col] > rows[j][col]
	})

	return rows
}

// Shortcuts lists the keys of the page, the decoders can be opened unless a search or a
// command bar like sorting is in use.
func (m *Model) Shortcuts() []statusbar.Shortcut {
	shortcuts := m.pageShortcuts()
	if m.search != nil || m.cmdBar.IsFocussed() {
		return shortcuts
	}
	return append(shortcuts, statusbar.Shortcut{Name: "Decoders", Keybinding: "d"})
}

func (m *Model) pageShortcuts() []statusbar.Shortcut {
	if m.search != nil {
		return m.search.Shortcuts()
	}
	if m.consuming {
		return m.withCopyShortcut([]statusbar.Shortcut{
			{"View Record", "enter"},
			{"Stop consuming", "F2"},
			{"New Search", "/"},
			{"Go Back", "esc"},
		})
	} else if m.noRecordsAvailable || m.noRecordsFound {
		return []statusbar.Shortcut{
			{"Search", "/"},
			{"Advanced Search", "C-f"},
			{"Go Back", "esc"},
		}
	} else if m.cmdBar.IsSorting() {
		return []statusbar.Shortcut{
			{"Cancel Sorting", "F3"},
			{"Select Sorting Column", "←/→/h/l"},
			{"Apply Sorting Column", "enter"},
		}
	}

	return m.withCopyShortcut([]statusbar.Shortcut{
		{"View Record", "enter"},
		{"Search", "/"},
		{"Advanced Search", "C-f"},
		{"Sort", "F3"},
		{"Go Back", "esc"},
	})
}

// withCopyShortcut adds the copy shortcut while the preview shows a payload.
func (m *Model) withCopyShortcut(shortcuts []statusbar.Shortcut) []statusbar.Shortcut {
	if record := m.selectedRecord(); m.preview.hasTransformation(record) {
		name := "Show Transformed"
		if m.preview.showOriginal {
			name = "Show Original"
		}
		shortcuts = append(shortcuts, statusbar.Shortcut{Name: name, Keybinding: "t"})
	}
	if !m.canCopy() {
		return shortcuts
	}
	return append(shortcuts[:1:1], append([]statusbar.Shortcut{{Name: "Copy Record", Keybinding: "c"}}, shortcuts[1:]...)...)
}

// avroSchemas are the Avro decoder IDs of the loaded records with the schema of their records.
func (m *Model) avroSchemas() map[string]string {
	schemas := map[string]string{}
	for _, record := range m.records {
		id := record.Payload.RuleID
		if strings.HasPrefix(id, "avro:") {
			if _, ok := schemas[id]; !ok {
				schemas[id] = record.Payload.Schema
			}
		}
	}
	return schemas
}

// SetTransformer applies the transformations of the topic to the records shown in the preview.
func (m *Model) SetTransformer(transformer *transformation.Transformer, topic string) {
	m.preview.transformer = transformer
	m.preview.topic = topic
}

func (m *Model) Title() string {
	return "Topics / " + m.readDetails.TopicName + " / Records"
}

// New creates the records page. With openSearch it opens the expanded search
// modal and waits for it to be submitted instead of reading right away.
func New(
	reader kadmin.RecordReader,
	protoDecoders *serdes.ProtobufDecoders,
	readDetails kadmin.ReadDetails,
	topic *kadmin.ListedTopic,
	openSearch bool,
	navigator tabs.TopicsTabNavigator,
) (pages.Page, tea.Cmd) {
	m := &Model{}

	t := table.New(
		table.WithFocused(true),
		table.WithStyles(styles.Table.Styles),
	)
	m.table = &t
	m.preview = newRecordPreview()
	m.clipWriter = clipper.New()
	m.reader = reader
	m.protoDecoders = protoDecoders
	m.cmdBar = NewConsumptionCmdbar()
	m.readDetails = readDetails
	m.topic = topic
	m.navigator = navigator
	m.cancelConsumption = func() {}

	m.border = border.New(
		border.WithInnerPaddingTop(),
		border.WithTitleFn(func() string {
			if m.progress != nil {
				return border.KeyValueTitle("Matches", fmt.Sprintf(" %d · searched %s of %s",
					len(m.rows), humanize.Comma(m.progress.Scanned()), humanize.Comma(m.progress.Total())), true)
			}
			return border.KeyValueTitle("Records", fmt.Sprintf(" %d", len(m.rows)), true)
		}))

	if openSearch {
		m.search = newSearchModal(topic, readDetails, true)
		return m, nil
	}
	return m, m.startReading(readDetails)
}

// startReading cancels any running consumption and reads the topic anew.
func (m *Model) startReading(details kadmin.ReadDetails) tea.Cmd {
	m.cancelConsumption()
	ctx, cancelFn := context.WithCancel(context.Background())
	m.cancelConsumption = cancelFn

	m.readDetails = details
	m.readStarted = true
	m.decodedVersion = m.protoDecoders.Version()
	m.source = nil
	m.progress = nil
	m.selected = nil
	m.records = nil
	m.rows = nil
	m.noRecordsAvailable = false
	m.noRecordsFound = false
	m.goToTop = true

	return func() tea.Msg {
		msg := m.reader.ReadRecords(ctx, details)
		if ctx.Err() != nil {
			// replaced by a newer search while starting
			return nil
		}
		return msg
	}
}
