package transformation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransform(t *testing.T) {
	const order = `{"id": "o-1", "items": [{"price": 2}, {"price": 3.5}], "big": 12345678901234567890}`

	t.Run("selects and computes fields", func(t *testing.T) {
		out, err := Transform(`{id, total: (.items | map(.price) | add)}`, order)

		require.NoError(t, err)
		assert.JSONEq(t, `{"id": "o-1", "total": 5.5}`, out)
	})

	t.Run("keeps large numbers exact", func(t *testing.T) {
		out, err := Transform(`.big`, order)

		require.NoError(t, err)
		assert.Equal(t, "12345678901234567890", out)
	})

	t.Run("rejects an invalid filter", func(t *testing.T) {
		_, err := Transform(`.id |`, order)

		assert.ErrorContains(t, err, "invalid filter")
	})

	t.Run("reports a runtime error", func(t *testing.T) {
		_, err := Transform(`.id + 1`, order)

		assert.Error(t, err)
	})

	t.Run("reports a filter without output", func(t *testing.T) {
		_, err := Transform(`empty`, order)

		assert.ErrorContains(t, err, "no output")
	})

	t.Run("rejects a document that is not JSON", func(t *testing.T) {
		_, err := Transform(`.`, "not json")

		assert.ErrorContains(t, err, "not JSON")
	})
}
