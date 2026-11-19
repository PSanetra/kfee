package record_details_page

import (
	"fmt"
	"kfee/config"
	"kfee/kadmin"
	"kfee/serdes"
	"kfee/styles"
	"kfee/ui/components/dirpicker"
	"kfee/ui/components/statusbar"
	"os"
	"regexp"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	lg "charm.land/lipgloss/v2"
)

const maxProtoModalWidth = 80

type protoStage int

const (
	stageAction protoStage = iota
	stageMode
	stageDir
	stageCompiling
	stageMessageType
	stageHeader
	stageRegex
)

const (
	actionAddRule = "add"
	actionRemove  = "remove"
)

const (
	modeSchema = "schema"
	modeRaw    = "raw"
)

// protoDecoderValues holds what the user entered, the huh fields write into it.
type protoDecoderValues struct {
	action      string
	mode        string
	dir         string
	messageType string
	header      string
	regex       string
}

// protoDecoderModal configures the protobuf decoder of a topic in a few steps:
// choose the directory with the .proto files, choose the message type and optionally
// choose a header of the record, whose value (as regex) selects the message type.
type protoDecoderModal struct {
	topic    string
	record   *kadmin.ConsumerRecord
	existing *config.ProtobufTopicConfig
	protoc   string
	compile  serdes.ProtoCompiler
	stage    protoStage
	values   protoDecoderValues
	schema   *serdes.ProtoSchema
	form     *huh.Form
	err      string
	// dirs chooses the proto directory, recentDirs are the proto directories of other topics
	dirs       *dirpicker.Model
	recentDirs []string
}

// protoSchemaCompiledMsg is the result of compiling the .proto files of dir.
type protoSchemaCompiledMsg struct {
	dir    string
	schema *serdes.ProtoSchema
	err    error
}

// protoDecoderSavedMsg is returned by the modal when the user confirms the decoder.
type protoDecoderSavedMsg struct {
	topic  config.ProtobufTopicConfig
	schema *serdes.ProtoSchema
}

// protoDecoderRemovedMsg is returned by the modal when the user removes the decoder of the topic.
type protoDecoderRemovedMsg struct {
	topic string
}

// protoDecoderClosedMsg is returned by the modal when the user cancels.
type protoDecoderClosedMsg struct{}

func newProtoDecoderModal(
	topic string,
	record *kadmin.ConsumerRecord,
	protobufCfg *config.ProtobufConfig,
	protoc string,
	compile serdes.ProtoCompiler,
	preselectedHeader string,
) *protoDecoderModal {
	m := &protoDecoderModal{
		topic:   topic,
		record:  record,
		protoc:  protoc,
		compile: compile,
		values: protoDecoderValues{
			action:      actionAddRule,
			mode:        modeSchema,
			dir:         initialProtoDir(topic, protobufCfg),
			messageType: record.Payload.MessageType,
			header:      preselectedHeader,
		},
	}
	if record.Payload.MessageType == serdes.RawMessageType {
		m.values.mode = modeRaw
	}
	if protobufCfg != nil {
		for _, t := range protobufCfg.Topics {
			if t.ProtoDir != "" {
				m.recentDirs = append(m.recentDirs, t.ProtoDir)
			}
		}
	}
	if existing := protobufCfg.Topic(topic); existing != nil {
		cfg := *existing
		m.existing = &cfg
		m.toStage(stageAction)
	} else {
		m.toStage(stageMode)
	}
	return m
}

// initialProtoDir prefers the topic's proto directory, then the one of another topic,
// then the working directory.
func initialProtoDir(topic string, cfg *config.ProtobufConfig) string {
	if t := cfg.Topic(topic); t != nil && t.ProtoDir != "" {
		return t.ProtoDir
	}
	if cfg != nil {
		for _, t := range cfg.Topics {
			if t.ProtoDir != "" {
				return t.ProtoDir
			}
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

func (m *protoDecoderModal) toStage(stage protoStage) {
	m.stage = stage
	m.err = ""
	v := &m.values

	var field huh.Field
	switch stage {
	case stageAction:
		field = huh.NewSelect[string]().
			Value(&v.action).
			Title("Protobuf decoder").
			Description(fmt.Sprintf("Topic %s already has a protobuf decoder", m.topic)).
			Options(
				huh.NewOption("Add or replace a rule", actionAddRule),
				huh.NewOption("Remove the decoder of this topic", actionRemove),
			)
	case stageMode:
		field = huh.NewSelect[string]().
			Value(&v.mode).
			Title("Decode with").
			Options(
				huh.NewOption(".proto files (field names and types)", modeSchema),
				huh.NewOption("Without schema (field numbers only, types guessed)", modeRaw),
			)
	case stageDir:
		m.form = nil
		m.dirs = dirpicker.New(v.dir, ".proto", m.recentDirs)
		return
	case stageCompiling:
		m.form = nil
		return
	case stageMessageType:
		var options []huh.Option[string]
		for _, t := range m.schema.MessageTypes {
			options = append(options, huh.NewOption(t, t))
		}
		if !m.hasMessageType(v.messageType) {
			v.messageType = m.defaultMessageType()
		}
		field = huh.NewSelect[string]().
			Value(&v.messageType).
			Title("Message type").
			Description("Press / to filter").
			Height(min(len(options)+3, 14)).
			Options(options...)
	case stageHeader:
		options := []huh.Option[string]{huh.NewOption("None, use this for all other records", "")}
		for _, key := range m.headerKeys() {
			options = append(options, huh.NewOption(fmt.Sprintf("%s = %s", key, truncate(m.headerValue(key), 40)), key))
		}
		field = huh.NewSelect[string]().
			Value(&v.header).
			Title("Discriminator header").
			Height(min(len(options)+3, 14)).
			Options(options...)
	case stageRegex:
		v.regex = m.existingRegex()
		if v.regex == "" {
			v.regex = "^" + regexp.QuoteMeta(m.headerValue(v.header)) + "$"
		}
		field = huh.NewInput().
			Value(&v.regex).
			Title("Value regex of header " + v.header)
	}

	m.form = huh.NewForm(huh.NewGroup(field)).WithTheme(styles.Huh).WithShowHelp(false)
	m.form.Init()
}

func (m *protoDecoderModal) decodedAs() string {
	if m.values.mode == modeRaw {
		return "without schema"
	}
	return "as " + m.values.messageType
}

func (m *protoDecoderModal) hasMessageType(messageType string) bool {
	for _, t := range m.schema.MessageTypes {
		if t == messageType {
			return true
		}
	}
	return false
}

// defaultMessageType is the type of the existing fallback rule or the first message type.
func (m *protoDecoderModal) defaultMessageType() string {
	if m.existing != nil {
		for _, r := range m.existing.Rules {
			if r.IsFallback() && m.hasMessageType(r.MessageType) {
				return r.MessageType
			}
		}
	}
	if len(m.schema.MessageTypes) > 0 {
		return m.schema.MessageTypes[0]
	}
	return ""
}

// existingRegex returns the regex of an existing rule of the chosen header and message type.
func (m *protoDecoderModal) existingRegex() string {
	if m.existing == nil {
		return ""
	}
	raw := m.values.mode == modeRaw
	for _, r := range m.existing.Rules {
		if r.Header == m.values.header && r.Raw == raw && (raw || r.MessageType == m.values.messageType) {
			return r.ValueRegex
		}
	}
	return ""
}

func (m *protoDecoderModal) headerKeys() []string {
	var keys []string
	seen := map[string]bool{}
	for _, h := range m.record.Headers {
		if !seen[h.Key] {
			seen[h.Key] = true
			keys = append(keys, h.Key)
		}
	}
	return keys
}

func (m *protoDecoderModal) headerValue(key string) string {
	for _, h := range m.record.Headers {
		if h.Key == key {
			return string(h.Value.Bytes())
		}
	}
	return ""
}

func (m *protoDecoderModal) headerMatches(key string, regex *regexp.Regexp) bool {
	for _, h := range m.record.Headers {
		if h.Key == key && regex.Match(h.Value.Bytes()) {
			return true
		}
	}
	return false
}

func truncate(s string, length int) string {
	runes := []rune(s)
	if len(runes) <= length {
		return s
	}
	return string(runes[:length-1]) + "…"
}

func (m *protoDecoderModal) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case protoSchemaCompiledMsg:
		if m.stage != stageCompiling || msg.dir != m.values.dir {
			return nil
		}
		if msg.err != nil {
			m.toStage(stageDir)
			m.err = msg.err.Error()
			return nil
		}
		if len(msg.schema.MessageTypes) == 0 {
			m.toStage(stageDir)
			m.err = "no message types found in " + msg.schema.Dir
			return nil
		}
		m.schema = msg.schema
		m.toStage(stageMessageType)
		return nil
	case tea.KeyPressMsg:
		if m.stage == stageDir {
			return m.updateDirs(msg)
		}
		m.err = ""
		switch msg.String() {
		case "esc":
			// esc ends filtering of a select instead of closing
			if !m.isFiltering() {
				return func() tea.Msg { return protoDecoderClosedMsg{} }
			}
		case "enter":
			if m.form == nil {
				return nil
			}
			// let the focused field take over the highlighted value
			m.updateForm(msg)
			return m.next()
		}
	}

	if m.form == nil {
		return nil
	}
	return m.updateForm(msg)
}

func (m *protoDecoderModal) updateDirs(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "esc" {
		// esc clears the filter before closing
		if m.dirs.Filtering() {
			m.dirs.ClearFilter()
			return nil
		}
		return func() tea.Msg { return protoDecoderClosedMsg{} }
	}
	m.err = ""
	if chosen := m.dirs.Update(msg); chosen {
		m.values.dir = m.dirs.Dir()
		return m.next()
	}
	return nil
}

func (m *protoDecoderModal) updateForm(msg tea.Msg) tea.Cmd {
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}
	return cmd
}

func (m *protoDecoderModal) isFiltering() bool {
	if m.form == nil {
		return false
	}
	if s, ok := m.form.GetFocusedField().(*huh.Select[string]); ok {
		return s.GetFiltering()
	}
	return false
}

// next validates the current step and moves on to the next one.
func (m *protoDecoderModal) next() tea.Cmd {
	v := &m.values
	switch m.stage {
	case stageAction:
		if v.action == actionRemove {
			topic := m.topic
			return func() tea.Msg { return protoDecoderRemovedMsg{topic: topic} }
		}
		m.toStage(stageMode)
	case stageMode:
		if v.mode == modeRaw {
			return m.afterDecoding()
		}
		m.toStage(stageDir)
	case stageDir:
		info, err := os.Stat(serdes.ExpandHome(v.dir))
		if err != nil || !info.IsDir() {
			m.err = v.dir + " is not a directory"
			return nil
		}
		m.toStage(stageCompiling)
		dir, protoc, compile := v.dir, m.protoc, m.compile
		return func() tea.Msg {
			schema, err := compile(protoc, dir)
			return protoSchemaCompiledMsg{dir: dir, schema: schema, err: err}
		}
	case stageMessageType:
		if v.messageType == "" {
			m.err = "choose a message type"
			return nil
		}
		return m.afterDecoding()
	case stageHeader:
		if v.header == "" {
			return m.save()
		}
		m.toStage(stageRegex)
	case stageRegex:
		regex, err := regexp.Compile(v.regex)
		if err != nil {
			m.err = "invalid regex: " + err.Error()
			return nil
		}
		if !m.headerMatches(v.header, regex) {
			m.err = "the regex does not match the " + v.header + " header of this record"
			return nil
		}
		return m.save()
	}
	return nil
}

// afterDecoding continues with the discriminator header, if the record has headers.
func (m *protoDecoderModal) afterDecoding() tea.Cmd {
	if len(m.record.Headers) == 0 {
		m.values.header = ""
		return m.save()
	}
	m.toStage(stageHeader)
	return nil
}

func (m *protoDecoderModal) save() tea.Cmd {
	v := m.values
	topicCfg := config.ProtobufTopicConfig{Topic: m.topic}
	if m.existing != nil {
		topicCfg = *m.existing
	}
	var rule config.ProtobufRule
	if v.mode == modeRaw {
		// keeps the proto directory of other rules
		rule.Raw = true
	} else {
		topicCfg.ProtoDir = v.dir
		rule.MessageType = v.messageType
	}
	if v.header != "" {
		rule.Header, rule.ValueRegex = v.header, v.regex
	}
	topicCfg = topicCfg.WithRule(rule)

	schema := m.schema
	return func() tea.Msg { return protoDecoderSavedMsg{topic: topicCfg, schema: schema} }
}

// View renders the modal sized to fit within the given window width.
func (m *protoDecoderModal) View(windowWidth int) string {
	width := min(maxProtoModalWidth, max(20, windowWidth-8))

	parts := []string{styles.Help.Title.Render("Protobuf decoder for " + m.topic), ""}
	switch {
	case m.stage == stageDir:
		parts = append(parts,
			styles.Help.Key.Render("Proto directory"),
			styles.Help.Footer.Render("Searched recursively for .proto files, used as import path"),
			"",
			m.dirs.View(width))
	case m.form == nil:
		parts = append(parts, "⏳ Compiling .proto files in "+m.values.dir+" …")
	default:
		// descriptions are wrapped here: huh does not wrap them to the modal width,
		// which makes the form layout shift and clip its fields
		if desc := m.description(); desc != "" {
			parts = append(parts, styles.Help.Footer.Width(width).Render(desc))
		}
		m.form.WithWidth(width)
		parts = append(parts, m.form.View())
	}
	if m.err != "" {
		parts = append(parts, lg.NewStyle().Width(width).Render(
			styles.Notifier.Error.UnsetMarginLeft().Render(m.err)))
	}
	parts = append(parts, "", styles.Help.Footer.Width(width).Render(m.footer()))
	return styles.Help.Modal.Render(lg.JoinVertical(lg.Left, parts...))
}

// description explains the field of the current stage, empty if there is none.
func (m *protoDecoderModal) description() string {
	switch m.stage {
	case stageHeader:
		return "Decode records " + m.decodedAs() + " only when this header matches"
	case stageRegex:
		return "Records whose " + m.values.header + " header matches are decoded " + m.decodedAs()
	}
	return ""
}

func (m *protoDecoderModal) footer() string {
	switch m.stage {
	case stageDir:
		return "↑↓ move · → open · ← up · type to filter · ctrl+s use folder · esc close"
	case stageMessageType, stageHeader:
		return "enter select · / filter · esc close"
	case stageCompiling:
		return "esc close"
	default:
		return "enter next · esc close"
	}
}

func (m *protoDecoderModal) Shortcuts() []statusbar.Shortcut {
	if m.stage == stageDir {
		return []statusbar.Shortcut{
			{Name: "Use This Folder", Keybinding: "C-s"},
			{Name: "Open", Keybinding: "→/enter"},
			{Name: "Up", Keybinding: "←"},
			{Name: "Close", Keybinding: "esc"},
		}
	}
	return []statusbar.Shortcut{
		{Name: "Next", Keybinding: "enter"},
		{Name: "Close", Keybinding: "esc"},
	}
}
