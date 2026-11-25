package decoders_page

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kfee/config"
	"kfee/kontext"
	"kfee/tests"
	"kfee/transformation"
	"kfee/ui/clipper"
	"kfee/ui/tabs"
)

func TestDecodersBack(t *testing.T) {
	newPage := func(navigator tabs.TopicsTabNavigator) *Model {
		return New("orders", &config.Config{}, nil, nil, navigator, nil, clipper.NewMock())
	}

	t.Run("returns to the consumption page without a callback", func(t *testing.T) {
		navigator := tabs.NewMockTopicsTabNavigator()
		m := newPage(navigator)

		cmd := m.Update(tests.Key(tea.KeyEscape))

		assert.IsType(t, tabs.ToCachedConsumptionPageCalledMsg{}, cmd())
	})

	t.Run("returns to the page that opened the decoders", func(t *testing.T) {
		m := newPage(tabs.NewMockTopicsTabNavigator())
		called := false
		m.SetBack(func() tea.Cmd {
			called = true
			return nil
		})

		m.Update(tests.Key(tea.KeyEscape))

		assert.True(t, called)
	})
}

func TestShowOnlyDecoder(t *testing.T) {
	schema := `{"type":"record","name":"Order","fields":[{"name":"id","type":"string"}]}`
	newPage := func(navigator tabs.TopicsTabNavigator) *Model {
		return New("orders", &config.Config{}, nil, nil, navigator, map[string]string{"avro:orders-value": schema}, clipper.NewMock())
	}

	t.Run("an unknown decoder leaves the list", func(t *testing.T) {
		m := newPage(tabs.NewMockTopicsTabNavigator())
		assert.False(t, m.ShowOnly("nope"))
		assert.False(t, m.detail)
	})

	t.Run("shows the decoder in its detail view, and esc goes back instead of the list", func(t *testing.T) {
		m := newPage(tabs.NewMockTopicsTabNavigator())
		back := false
		m.SetBack(func() tea.Cmd {
			back = true
			return nil
		})

		require.True(t, m.ShowOnly("avro:orders-value"))
		assert.True(t, m.detail)

		m.Update(tests.Key(tea.KeyEscape))

		assert.True(t, back)
	})

	t.Run("the detail box spans the full height with few or many transformations", func(t *testing.T) {
		ktx := kontext.New(nil)
		ktx.WindowWidth, ktx.WindowHeight, ktx.AvailableHeight = 120, 40, 36
		for _, count := range []int{0, 2, 40} {
			m := newPage(tabs.NewMockTopicsTabNavigator())
			require.True(t, m.ShowOnly("avro:orders-value"))
			for i := 0; i < count; i++ {
				m.history["avro:orders-value"] = append(m.history["avro:orders-value"], transformation.Entry{Filter: "{id: .id}"})
			}

			lines := strings.Split(ansi.Strip(m.detailView(ktx)), "\n")

			assert.Len(t, lines, ktx.AvailableHeight-2, "transformations: %d", count)
		}
	})
}
