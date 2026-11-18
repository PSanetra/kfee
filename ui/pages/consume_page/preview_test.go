package consume_page

import (
	"ktea/kadmin"
	"ktea/serdes"
	"ktea/tests"
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

	t.Run("hides the preview in narrow windows", func(t *testing.T) {
		render := view(newPage(), 120)

		assert.NotContains(t, render, "Preview")
		assert.NotContains(t, render, `"second-order"`)
	})
}
