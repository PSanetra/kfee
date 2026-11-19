package consume_page

import (
	"testing"

	"kfee/kadmin"
	"kfee/serdes"
	"kfee/transformation"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordPreviewTransformation(t *testing.T) {
	store := transformation.Store{Dir: t.TempDir()}
	_, err := store.Append("prd", "orders", "r1", transformation.Entry{Source: transformation.SourceManual, Filter: `{kind: .reason}`})
	require.NoError(t, err)

	p := newRecordPreview()
	p.transformer = transformation.NewTransformer(store, "prd")
	p.topic = "orders"
	record := &kadmin.ConsumerRecord{
		Offset:  1,
		Payload: serdes.DesData{Value: `{"id": "o-1", "reason": "late"}`, RuleID: "r1"},
	}
	other := &kadmin.ConsumerRecord{
		Offset:  2,
		Payload: serdes.DesData{Value: `{"id": "o-2"}`, RuleID: "r2"},
	}

	t.Run("shows the transformed value of the rule", func(t *testing.T) {
		assert.Contains(t, ansi.Strip(p.payload(record)), `"kind": "late"`)
		assert.Equal(t, "transformed", p.transformMarker(record))
	})

	t.Run("toggles to the original value", func(t *testing.T) {
		p.toggleOriginal(record)

		assert.Contains(t, ansi.Strip(p.payload(record)), `"id": "o-1"`)
		assert.Equal(t, "original", p.transformMarker(record))
		assert.Contains(t, p.copyable(record), `"id": "o-1"`)
	})

	t.Run("records without transformation are shown as they are", func(t *testing.T) {
		assert.Equal(t, "", p.transformMarker(other))
		assert.Contains(t, ansi.Strip(p.payload(other)), `"id": "o-2"`)
	})
}
