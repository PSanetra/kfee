package consume_page

import (
	"kfee/kadmin"
	"kfee/styles"
	"kfee/ui"
	"kfee/ui/components/border"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// minWidthForPreview is the window width from which the selected record is previewed
	minWidthForPreview = 140
	previewWidthRatio  = 0.4
	// previewMaxLines is the number of payload lines rendered, more never fit into the preview
	previewMaxLines = 200
	// previewCacheSize is the number of rendered previews kept
	previewCacheSize = 64
)

// previewWidth returns the outer width of the preview of the selected record, 0 when there is not enough space.
func previewWidth(windowWidth int) int {
	if windowWidth < minWidthForPreview {
		return 0
	}
	return int(float64(windowWidth) * previewWidthRatio)
}

// recordPreview renders the payload of the selected record. Rendering large payloads takes a while,
// so it is done in the background while a spinner is shown, and the result is cached.
type recordPreview struct {
	border *border.Model
	record *kadmin.ConsumerRecord

	cache map[previewKey]string
	// order of the cached keys, the oldest first
	order []previewKey
	// rendering is the key rendered in the background, nil when nothing is rendered
	rendering *previewKey
	spinner   spinner.Model

	// copyStatus is shown in the title after copying the record until another record is selected
	copyStatus    string
	copyStatusKey previewKey
}

type previewKey struct {
	partition, offset int64
	value             string
	syntax            string
}

// previewRenderedMsg is the result of rendering a preview in the background.
type previewRenderedMsg struct {
	key      previewKey
	rendered string
}

func keyOf(record *kadmin.ConsumerRecord) previewKey {
	return previewKey{
		partition: record.Partition,
		offset:    record.Offset,
		value:     record.Payload.Value,
		syntax:    styles.Current.Syntax,
	}
}

func newRecordPreview() *recordPreview {
	p := &recordPreview{
		cache:   map[previewKey]string{},
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	p.border = border.New(
		border.WithTitleFn(func() string {
			if p.record == nil {
				return border.Title("Preview", false)
			}
			title := "Preview · " + p.record.PayloadType()
			if p.copyStatus != "" && p.copyStatusKey == keyOf(p.record) {
				title += " · " + p.copyStatus
			}
			return border.Title(title, false)
		}))
	p.border.Focused = false
	return p
}

// renderPreview formats and highlights the part of the payload shown in the preview.
func renderPreview(value string) string {
	// tabs have no defined width in the terminal
	return strings.ReplaceAll(ui.PrettyPrintJsonLines(value, previewMaxLines), "\t", "  ")
}

// request starts rendering the record in the background unless it is rendered already.
func (p *recordPreview) request(record *kadmin.ConsumerRecord) tea.Cmd {
	if record == nil || record.Err != nil {
		return nil
	}
	key := keyOf(record)
	if _, ok := p.cache[key]; ok {
		return nil
	}
	if p.rendering != nil && *p.rendering == key {
		return nil
	}
	startSpinner := p.rendering == nil
	p.rendering = &key
	value := record.Payload.Value
	render := func() tea.Msg {
		return previewRenderedMsg{key: key, rendered: renderPreview(value)}
	}
	if startSpinner {
		return tea.Batch(render, p.spinner.Tick)
	}
	return render
}

// Update handles the background rendering, it reports whether msg was handled.
func (p *recordPreview) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case previewRenderedMsg:
		p.store(msg.key, msg.rendered)
		if p.rendering != nil && *p.rendering == msg.key {
			p.rendering = nil
		}
		return nil, true
	case spinner.TickMsg:
		if msg.ID != p.spinner.ID() {
			return nil, false
		}
		// the spinner stops when nothing is rendered anymore
		if p.rendering == nil {
			return nil, true
		}
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return cmd, true
	}
	return nil, false
}

func (p *recordPreview) store(key previewKey, rendered string) {
	if _, ok := p.cache[key]; ok {
		return
	}
	p.cache[key] = rendered
	p.order = append(p.order, key)
	if len(p.order) > previewCacheSize {
		delete(p.cache, p.order[0])
		p.order = p.order[1:]
	}
}

// copied shows the result of copying the given record in the title.
func (p *recordPreview) copied(record *kadmin.ConsumerRecord, err error) {
	p.copyStatusKey = keyOf(record)
	if err != nil {
		p.copyStatus = "copy failed: " + err.Error()
	} else {
		p.copyStatus = "copied ✓"
	}
}

// View renders the preview with the given outer width and height.
func (p *recordPreview) View(record *kadmin.ConsumerRecord, width, height int) string {
	p.record = record
	// border and padding take two columns each
	innerWidth, innerHeight := max(width-4, 1), max(height-2, 1)
	muted := lipgloss.NewStyle().Foreground(styles.Current.TextMuted)

	var content string
	switch {
	case record == nil:
		content = muted.Render("No record selected")
	case record.Err != nil:
		content = muted.Width(innerWidth).Render("Unable to render payload: " + record.Err.Error())
	default:
		content = p.payload(record)
	}

	lines := strings.Split(content, "\n")
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, innerWidth, "…")
	}
	return p.border.View(lipgloss.NewStyle().
		Width(innerWidth+2).
		Height(innerHeight).
		MaxHeight(innerHeight).
		Padding(0, 1).
		Render(strings.Join(lines, "\n")))
}

func (p *recordPreview) payload(record *kadmin.ConsumerRecord) string {
	key := keyOf(record)
	if rendered, ok := p.cache[key]; ok {
		return rendered
	}
	if p.rendering != nil {
		return p.spinner.View() + lipgloss.NewStyle().Foreground(styles.Current.TextMuted).Render(" Rendering …")
	}
	// not requested, e.g. the selection changed while rendering the page; rendering only the
	// visible lines is fast enough to be done right away
	rendered := renderPreview(record.Payload.Value)
	p.store(key, rendered)
	return rendered
}

// copyablePayload is the payload as copied to the clipboard, the same as in the record details page.
func copyablePayload(record *kadmin.ConsumerRecord) string {
	return ui.FormatPayload(record.Payload.Value)
}
