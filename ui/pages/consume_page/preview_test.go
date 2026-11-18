package consume_page

import (
	"errors"
	"ktea/kadmin"
	"ktea/serdes"
	"ktea/tests"
	"ktea/ui/clipper"
	"ktea/ui/components/statusbar"
	"ktea/ui/tabs"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestRecordPreview(t *testing.T) {
	newPage := func() *Model {
		topic := &kadmin.ListedTopic{Name: "orders", PartitionCount: 1}
		page, _ := New(kadmin.NewMockKadmin(), nil, kadmin.NewDefaultReadDetails(topic), topic, false, tabs.NewMockTopicsTabNavigator())
		m := page.(*Model)
		now := time.Now()
		m.Update(kadmin.ConsumerRecordReceived{Records: []kadmin.ConsumerRecord{
			{Key: "k1", Offset: 1, Timestamp: now, Payload: serdes.DesData{Value: `{"order":"first-order"}`}},
			{Key: "k2", Offset: 2, Timestamp: now.Add(time.Second), Payload: serdes.DesData{Value: `{"order":"second-order"}`}},
		}})
		return m
	}
	view := func(m *Model, width int) string {
		ktx := tests.NewKontext(tests.WithWindowWidth(width))
		return ansi.Strip(m.View(ktx, tests.Renderer))
	}

	t.Run("previews the selected record in wide windows", func(t *testing.T) {
		m := newPage()

		// sorted by timestamp descending, the newest record is selected first
		render := view(m, 160)
		assert.Contains(t, render, "Preview · Plain Json")
		assert.Contains(t, render, `"second-order"`)
		assert.NotContains(t, render, `"first-order"`)

		m.Update(tests.Key(tea.KeyDown))
		render = view(m, 160)
		assert.Contains(t, render, `"first-order"`)
	})

	t.Run("fits the window", func(t *testing.T) {
		for _, line := range strings.Split(view(newPage(), 160), "\n") {
			assert.LessOrEqual(t, lipgloss.Width(line), 160, line)
		}
	})

	t.Run("shows the search progress while searching", func(t *testing.T) {
		m := newPage()

		m.Update(&kadmin.ReadingStartedMsg{Progress: &kadmin.SearchProgress{}})

		assert.Contains(t, view(m, 120), "Matches:  2 · searched 0 of 0")
		m.startReading(kadmin.NewDefaultReadDetails(m.topic))
		assert.Contains(t, view(m, 120), "Records:  0", "a new read without search shows the records again")
	})

	t.Run("hides the preview in narrow windows", func(t *testing.T) {
		render := view(newPage(), 120)

		assert.NotContains(t, render, "Preview")
		assert.NotContains(t, render, `"second-order"`)
	})

	t.Run("c copies the previewed record", func(t *testing.T) {
		m := newPage()
		var copied string
		m.clipWriter = &clipper.MockClipper{WriteFunc: func(text string) error {
			copied = text
			return nil
		}}
		view(m, 160)
		assert.Contains(t, m.Shortcuts(), statusbar.Shortcut{Name: "Copy Record", Keybinding: "c"})

		m.Update(tests.Key('c'))

		assert.JSONEq(t, `{"order":"second-order"}`, copied)
		assert.NotContains(t, copied, "\x1b", "no color codes")
		assert.Contains(t, view(m, 160), "Preview · Plain Json · copied ✓")

		// the status belongs to the copied record only
		m.Update(tests.Key(tea.KeyDown))
		assert.NotContains(t, view(m, 160), "copied")
	})

	t.Run("shows copy errors", func(t *testing.T) {
		m := newPage()
		m.clipWriter = &clipper.MockClipper{WriteFunc: func(string) error { return errors.New("no clipboard") }}
		view(m, 160)

		m.Update(tests.Key('c'))

		assert.Contains(t, view(m, 160), "copy failed: no clipboard")
	})

	t.Run("c does nothing without preview", func(t *testing.T) {
		m := newPage()
		copied := false
		m.clipWriter = &clipper.MockClipper{WriteFunc: func(string) error {
			copied = true
			return nil
		}}
		view(m, 120)

		m.Update(tests.Key('c'))

		assert.False(t, copied)
		assert.NotContains(t, m.Shortcuts(), statusbar.Shortcut{Name: "Copy Record", Keybinding: "c"})
	})
}
