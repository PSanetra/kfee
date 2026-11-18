package consume_page

import (
	"ktea/kadmin"
	"ktea/styles"
	"ktea/ui"
	"ktea/ui/components/border"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// minWidthForPreview is the window width from which the selected record is previewed
	minWidthForPreview = 140
	previewWidthRatio  = 0.4
)

// previewWidth returns the outer width of the preview of the selected record, 0 when there is not enough space.
func previewWidth(windowWidth int) int {
	if windowWidth < minWidthForPreview {
		return 0
	}
	return int(float64(windowWidth) * previewWidthRatio)
}

// recordPreview renders the payload of the selected record, cached as rendering is expensive.
type recordPreview struct {
	border *border.Model
	record *kadmin.ConsumerRecord

	cacheKey previewKey
	rendered string
}

type previewKey struct {
	partition, offset int64
	value             string
	syntax            string
}

func newRecordPreview() *recordPreview {
	p := &recordPreview{}
	p.border = border.New(
		border.WithTitleFn(func() string {
			if p.record == nil {
				return border.Title("Preview", false)
			}
			return border.Title("Preview · "+p.record.PayloadType(), false)
		}))
	p.border.Focused = false
	return p
}

// View renders the preview with the given outer width and height.
func (p *recordPreview) View(record *kadmin.ConsumerRecord, width, height int) string {
	p.record = record
	// border and padding take two columns each
	innerWidth, innerHeight := max(width-4, 1), max(height-2, 1)

	var content string
	switch {
	case record == nil:
		content = lipgloss.NewStyle().Foreground(styles.Current.TextMuted).Render("No record selected")
	case record.Err != nil:
		content = lipgloss.NewStyle().Width(innerWidth).Foreground(styles.Current.TextMuted).
			Render("Unable to render payload: " + record.Err.Error())
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
	key := previewKey{
		partition: record.Partition,
		offset:    record.Offset,
		value:     record.Payload.Value,
		syntax:    styles.Current.Syntax,
	}
	if key != p.cacheKey || p.rendered == "" {
		p.cacheKey = key
		// tabs have no defined width in the terminal
		p.rendered = strings.ReplaceAll(ui.PrettyPrintJson(record.Payload.Value), "\t", "  ")
	}
	return p.rendered
}
