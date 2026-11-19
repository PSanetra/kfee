package transformation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformer(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir, Limit: 10}
	const document = `{"id": "o-1", "status": 2}`

	_, err := store.Append("prd", "orders", "r1", Entry{Source: SourceManual, Filter: `{order: .id}`})
	require.NoError(t, err)
	_, err = store.Append("prd", "orders", "r2", Entry{Source: SourceLLM, Filter: `.broken |`})
	require.NoError(t, err)
	transformer := NewTransformer(store, "prd")

	t.Run("transforms with the latest filter of the rule", func(t *testing.T) {
		out, ok, err := transformer.Transform("orders", "r1", document)

		require.NoError(t, err)
		assert.True(t, ok)
		assert.JSONEq(t, `{"order": "o-1"}`, out)
	})

	t.Run("reports rules without transformation", func(t *testing.T) {
		_, ok, err := transformer.Transform("orders", "r3", document)

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("ignores records without rule", func(t *testing.T) {
		_, ok, err := transformer.Transform("orders", "", document)

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("ignores stored filters that do not compile", func(t *testing.T) {
		_, ok, err := transformer.Transform("orders", "r2", document)

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("reads new transformations after invalidation", func(t *testing.T) {
		_, err := store.Append("prd", "orders", "r3", Entry{Source: SourceManual, Filter: `.status`})
		require.NoError(t, err)

		_, ok, _ := transformer.Transform("orders", "r3", document)
		assert.False(t, ok, "cached until invalidated")

		transformer.Invalidate("orders")
		out, ok, err := transformer.Transform("orders", "r3", document)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "2", out)
	})
}
