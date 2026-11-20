package record_details_page

import (
	"kfee/tests"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// focusFilter moves from the description to the filter field, huh moves focus with a command.
func focusFilter(e *transformEditor) {
	if cmd := e.update(tests.Key(tea.KeyTab)); cmd != nil {
		e.update(cmd())
	}
}

func TestTransformEditorPreview(t *testing.T) {
	const payload = `{"id": "o-1", "reason": "late"}`

	t.Run("typing in the description does not run the filter", func(t *testing.T) {
		e := newTransformEditor(payload, ".id")
		before := e.previewOut
		seq := e.previewSeq

		e.update(tests.Key('x'))

		assert.Equal(t, "x", e.description)
		assert.Equal(t, ".id", e.filter)
		assert.Equal(t, seq, e.previewSeq)
		assert.Equal(t, before, e.previewOut)
	})

	t.Run("a changed filter is computed once the delay has passed", func(t *testing.T) {
		e := newTransformEditor(payload, ".id")
		require.Contains(t, e.previewOut, `"o-1"`)
		focusFilter(e)

		cmd := e.update(tests.Key('x'))
		require.NotNil(t, cmd, "a delayed computation is scheduled")
		seq := e.previewSeq
		assert.Contains(t, e.previewOut, `"o-1"`, "result stays until the delay has passed")

		e.handlePreviewDue(previewDueMsg{seq: seq})

		assert.Equal(t, ".idx", e.filter)
		assert.Equal(t, "null", e.previewOut)
		assert.NoError(t, e.previewErr)
	})

	t.Run("a stale delayed computation is ignored", func(t *testing.T) {
		e := newTransformEditor(payload, ".id")
		focusFilter(e)
		e.update(tests.Key('x'))
		stale := e.previewSeq
		e.update(tests.Key('y'))
		before := e.previewOut

		e.handlePreviewDue(previewDueMsg{seq: stale})

		assert.Equal(t, before, e.previewOut)
	})

	t.Run("rendering does not run the filter", func(t *testing.T) {
		e := newTransformEditor(payload, ".id")
		before := e.previewOut
		// the filter changed without the editor being told, rendering must keep the result
		e.filter = `.reason`

		e.View(120, 40)

		assert.Equal(t, before, e.previewOut)
	})
}
