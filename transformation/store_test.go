package transformation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreAppendKeepsLastEntries(t *testing.T) {
	store := Store{Dir: t.TempDir(), Limit: 2}
	at := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	entry := func(filter string) Entry {
		return Entry{Applied: at, Source: SourceManual, Filter: filter}
	}

	_, err := store.Append("prd", "orders", "r1", entry("a"))
	require.NoError(t, err)
	_, err = store.Append("prd", "orders", "r1", entry("b"))
	require.NoError(t, err)
	history, err := store.Append("prd", "orders", "r1", entry("c"))
	require.NoError(t, err)

	assert.Equal(t, []Entry{entry("b"), entry("c")}, history)
	loaded, err := store.Load("prd", "orders", "r1")
	require.NoError(t, err)
	assert.Equal(t, history, loaded)
}

func TestStoreKeepsRulesApart(t *testing.T) {
	store := Store{Dir: t.TempDir(), Limit: 10}
	_, err := store.Append("prd", "orders", "r1", Entry{Source: SourceLLM, Filter: ".a"})
	require.NoError(t, err)

	other, err := store.Load("prd", "orders", "r2")
	require.NoError(t, err)
	assert.Empty(t, other)
	otherTopic, err := store.Load("prd", "payments", "r1")
	require.NoError(t, err)
	assert.Empty(t, otherTopic)
	otherCluster, err := store.Load("dev", "orders", "r1")
	require.NoError(t, err)
	assert.Empty(t, otherCluster)
}

func TestStoreFileLayout(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir, Limit: 10}
	_, err := store.Append("prd", "orders", "r1", Entry{Filter: "."})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, "prd", "orders.json"))
}

func TestStoreEscapesNames(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir, Limit: 10}
	_, err := store.Append("a/b", "..", "r1", Entry{Filter: "."})
	require.NoError(t, err)

	// nothing is written outside of dir
	entries, err := os.ReadDir(filepath.Dir(dir))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, "..", e.Name())
	}
	loaded, err := store.Load("a/b", "..", "r1")
	require.NoError(t, err)
	assert.Len(t, loaded, 1)
	assert.Equal(t, "a%2Fb", filepath.Base(filepath.Dir(store.file("a/b", ".."))))
	assert.Equal(t, "%2E%2E.json", filepath.Base(store.file("a/b", "..")))
}

func TestStoreMissingFileIsEmpty(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "missing"), Limit: 10}

	loaded, err := store.Load("prd", "orders", "r1")

	require.NoError(t, err)
	assert.Empty(t, loaded)
}

func TestStoreActivateMovesEntryToLatest(t *testing.T) {
	store := Store{Dir: t.TempDir(), Limit: 10}
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, f := range []string{"a", "b", "c"} {
		_, err := store.Append("prd", "orders", "r1", Entry{Applied: old, Source: SourceManual, Filter: f})
		require.NoError(t, err)
	}

	history, err := store.Activate("prd", "orders", "r1", 0)

	require.NoError(t, err)
	require.Len(t, history, 3)
	assert.Equal(t, "b", history[0].Filter)
	assert.Equal(t, "c", history[1].Filter)
	assert.Equal(t, "a", history[2].Filter)
	assert.True(t, history[2].Applied.After(old))

	_, err = store.Activate("prd", "orders", "r1", 3)
	assert.Error(t, err)
}
