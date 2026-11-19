package record_details_page

import (
	"context"
	"errors"
	"kfee/config"
	"kfee/llm"
	"kfee/serdes"
	"kfee/transformation"
	"kfee/ui"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// transformationStore is the store of the applied transformations, limited to the configured history.
func (m *Model) transformationStore() transformation.Store {
	return transformation.Store{Dir: config.TransformationsDir(), Limit: m.config.TransformationHistoryLimit()}
}

// refreshTransformation applies the latest transformation of the decoder rule that decoded the
// record. The transformed payload replaces the original in the view, unless the user toggled it.
func (m *Model) refreshTransformation() {
	m.transformed = ""
	if m.err != nil || m.record == nil || m.record.Payload.RuleID == "" || m.config == nil {
		return
	}
	cluster := m.config.ActiveCluster()
	if cluster == nil {
		return
	}

	history, err := m.transformationStore().Load(cluster.Name, m.topicName, m.record.Payload.RuleID)
	if err != nil {
		m.notifierCmdbar.Notifier.ShowError(err)
		return
	}
	if len(history) == 0 {
		return
	}

	out, err := transformation.Transform(history[len(history)-1].Filter, m.record.Payload.Value)
	if err != nil {
		m.notifierCmdbar.Notifier.ShowError(err)
		return
	}
	m.transformed = ui.PrettyPrintJson(out)
}

// shownPayload is the payload as displayed: the transformed one unless the original is shown.
func (m *Model) shownPayload() string {
	if m.transformed != "" && !m.showOriginal {
		return m.transformed
	}
	return m.payload
}

// toggleOriginal switches between the transformed and the original payload.
func (m *Model) toggleOriginal() {
	if m.transformed == "" {
		return
	}
	m.showOriginal = !m.showOriginal
	m.resetViews()
}

// transformationMarker tells in the title which payload is shown, empty if there is no transformation.
func (m *Model) transformationMarker() string {
	switch {
	case m.transformed == "":
		return ""
	case m.showOriginal:
		return " [ original ]"
	default:
		return " [ transformed ]"
	}
}

// transformToggleName is the label of the key switching between the payloads.
func (m *Model) transformToggleName() string {
	if m.showOriginal {
		return "Show Transformed"
	}
	return "Show Original"
}

// openTransformEditor edits the transformation of the decoder rule that decoded the record.
func (m *Model) openTransformEditor() {
	if m.err != nil || m.record.Payload.RuleID == "" {
		m.notifierCmdbar.Notifier.ShowError(errors.New("only records decoded by a protobuf rule can be transformed"))
		return
	}
	cluster := m.config.ActiveCluster()
	if cluster == nil {
		return
	}
	history, err := m.transformationStore().Load(cluster.Name, m.topicName, m.record.Payload.RuleID)
	if err != nil {
		m.notifierCmdbar.Notifier.ShowError(err)
		return
	}
	m.transformEditor = newTransformEditor(m.record.Payload.Value, transformEditorFilter(history))
}

// updateTransformEditor handles the keys of the open editor, all other keys go to its form.
func (m *Model) updateTransformEditor(msg tea.KeyPressMsg) tea.Cmd {
	if m.transformEditor.picker != nil {
		return m.updateToolPicker(msg)
	}
	if m.transformEditor.files != nil {
		return m.updateFilePicker(msg)
	}
	switch msg.String() {
	case "esc":
		m.transformEditor = nil
		return nil
	case "ctrl+s":
		return m.saveTransformation()
	case "ctrl+o":
		m.transformEditor.openFilePicker()
		return nil
	case "ctrl+r":
		m.transformEditor.detachLast()
		return nil
	case "ctrl+g":
		return m.generateTransformation()
	case "ctrl+e":
		return m.openExternalEditor()
	}
	return m.transformEditor.update(msg)
}

// openExternalEditor edits the focused text field in $EDITOR. Its text is written to a temporary
// file, the result is applied to that field only, when the editor returns.
func (m *Model) openExternalEditor() tea.Cmd {
	e := m.transformEditor
	target, text, ok := e.focusedText()
	if !ok {
		return nil
	}
	path, err := writeTempFile(text)
	if err != nil {
		m.notifierCmdbar.Notifier.ShowError(err)
		return nil
	}
	name, args := externalEditor()
	cmd := exec.Command(name, append(args, path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return externalEditedMsg{target: target, err: err}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return externalEditedMsg{target: target, err: err}
		}
		return externalEditedMsg{target: target, text: strings.TrimSuffix(string(content), "\n")}
	})
}

// handleExternalEdited puts the text returned by the editor into its field. The other field keeps
// its text.
func (m *Model) handleExternalEdited(msg externalEditedMsg) tea.Cmd {
	e := m.transformEditor
	if e == nil {
		// the editor was closed while $EDITOR was open
		return nil
	}
	if msg.err != nil {
		e.status = "Editor failed: " + msg.err.Error()
		return nil
	}
	switch msg.target {
	case editFilter:
		e.setFilter(msg.text)
		return e.focusFilter()
	default:
		// huh does not follow values changed outside its fields, the form is rebuilt
		e.description = msg.text
		e.buildForm()
		return nil
	}
}

// externalEditor is the command of $EDITOR, nano when it is not set, as the text fields of huh.
func externalEditor() (string, []string) {
	fields := strings.Fields(os.Getenv("EDITOR"))
	if len(fields) > 0 {
		return fields[0], fields[1:]
	}
	return "nano", nil
}

// writeTempFile writes the text to a new temporary file and returns its path.
func writeTempFile(text string) (string, error) {
	file, err := os.CreateTemp("", "kfee-transform-*.txt")
	if err != nil {
		return "", err
	}
	if _, err := file.WriteString(text); err != nil {
		file.Close()
		os.Remove(file.Name())
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

// updateToolPicker handles the keys of the picker choosing the LLM tool.
func (m *Model) updateToolPicker(msg tea.KeyPressMsg) tea.Cmd {
	e := m.transformEditor
	p := e.picker
	if p.custom {
		switch msg.String() {
		case "esc":
			p.custom = false
			return nil
		case "enter":
			command := strings.Fields(p.input.Value())
			if len(command) == 0 {
				return nil
			}
			return m.chooseLLM(command, false)
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd
	}

	switch msg.String() {
	case "esc":
		e.picker = nil
	case "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down":
		if p.cursor < len(config.LLMPresets) {
			p.cursor++
		}
	case "enter":
		if p.cursor == len(config.LLMPresets) {
			p.custom = true
			return nil
		}
		preset := config.LLMPresets[p.cursor]
		return m.chooseLLM(preset.Config.Command, preset.Config.PromptAsArgument)
	}
	return nil
}

// updateFilePicker handles the keys of the picker choosing an attachment.
func (m *Model) updateFilePicker(msg tea.KeyPressMsg) tea.Cmd {
	e := m.transformEditor
	if msg.String() == "esc" {
		e.files = nil
		return nil
	}
	if chosen := e.files.Update(msg); chosen != "" {
		e.attach(chosen)
		e.files = nil
	}
	return nil
}

// chooseLLM stores the command generating transformations and generates one for the editor.
func (m *Model) chooseLLM(command []string, promptAsArgument bool) tea.Cmd {
	m.config.SetLLMCommand(command, promptAsArgument)
	m.transformEditor.picker = nil
	return m.generateTransformation()
}

// describeSchema describes the message type of the record for the prompt of the LLM.
func (m *Model) describeSchema() (string, error) {
	if m.record.Payload.MessageType == serdes.RawMessageType {
		return "Protobuf without schema: the keys of the JSON are field numbers.", nil
	}
	return m.protoDecoders.Describe(m.topicName, m.record.Payload.MessageType)
}

// generateTransformation asks the configured LLM for a filter matching the description. The
// filter is only put into the editor, it is saved when the user does so.
func (m *Model) generateTransformation() tea.Cmd {
	e := m.transformEditor
	if e.generating {
		return nil
	}
	if !m.config.LLM.Configured() {
		e.picker = newToolPicker()
		return nil
	}
	description := strings.TrimSpace(e.description)
	if description == "" {
		e.status = "Describe the transformation first."
		return nil
	}
	schema, err := m.describeSchema()
	if err != nil {
		m.notifierCmdbar.Notifier.ShowError(err)
		return nil
	}
	attachments, err := llm.ReadAttachments(e.attachments)
	if err != nil {
		m.notifierCmdbar.Notifier.ShowError(err)
		return nil
	}

	request := llm.Request{Description: description, Schema: schema, Attachments: attachments}
	if m.config.LLM.SendSample {
		request.Sample = m.record.Payload.Value
	}
	prompt := llm.BuildPrompt(request)
	command := *m.config.LLM

	e.generating = true
	e.status = ""
	return func() tea.Msg {
		out, err := llm.Run(context.Background(), command, prompt)
		if err != nil {
			return llmGeneratedMsg{err: err}
		}
		filter, err := llm.ParseFilter(out)
		return llmGeneratedMsg{filter: filter, err: err}
	}
}

// handleGenerated puts the generated filter into the editor, or shows why there is none.
func (m *Model) handleGenerated(msg llmGeneratedMsg) {
	e := m.transformEditor
	if e == nil {
		// the editor was closed while generating
		return
	}
	e.generating = false
	if msg.err != nil {
		e.status = "Generation failed: " + msg.err.Error()
		return
	}
	e.generated = msg.filter
	e.setFilter(msg.filter)
	e.status = "Generated. Check the result, then save with ctrl+s."
}

// saveTransformation applies the filter of the editor to the records of the rule from now on,
// it is kept in the history of the rule. Filters that fail on the shown record are not saved.
func (m *Model) saveTransformation() tea.Cmd {
	e := m.transformEditor
	filter := strings.TrimSpace(e.filter)
	if _, err := transformation.Transform(filter, m.record.Payload.Value); err != nil {
		m.notifierCmdbar.Notifier.ShowError(err)
		return nil
	}
	cluster := m.config.ActiveCluster()
	if cluster == nil {
		return nil
	}

	source := transformation.SourceManual
	if e.generated != "" && filter == strings.TrimSpace(e.generated) {
		source = transformation.SourceLLM
	}
	_, err := m.transformationStore().Append(cluster.Name, m.topicName, m.record.Payload.RuleID, transformation.Entry{
		Applied: time.Now(),
		Source:  source,
		Filter:  filter,
	})
	if err != nil {
		m.notifierCmdbar.Notifier.ShowError(err)
		return nil
	}

	m.transformEditor = nil
	if m.transformer != nil {
		m.transformer.Invalidate(m.topicName)
	}
	m.showOriginal = false
	m.refreshTransformation()
	m.resetViews()
	m.notifierCmdbar.Notifier.ShowSuccessMsg("Transformation saved")
	return m.notifierCmdbar.Notifier.AutoHideCmd("record-details-page")
}
