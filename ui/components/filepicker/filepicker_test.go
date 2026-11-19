package filepicker

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePicker(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "protos"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codes.txt"), []byte("1 = NEW"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644))
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	down := tea.KeyPressMsg{Code: tea.KeyDown}

	t.Run("lists directories before files and hides dotfiles", func(t *testing.T) {
		m := New(dir)

		require.Len(t, m.visible, 3)
		assert.Equal(t, "..", m.visible[0].label)
		assert.Equal(t, "protos/", m.visible[1].label)
		assert.Equal(t, "codes.txt", m.visible[2].label)
	})

	t.Run("enter opens a directory", func(t *testing.T) {
		m := New(dir)
		m.Update(down)

		assert.Empty(t, m.Update(enter))
		assert.Equal(t, filepath.Join(dir, "protos"), m.Dir())
	})

	t.Run("enter chooses a file", func(t *testing.T) {
		m := New(dir)
		m.Update(down)
		m.Update(down)

		assert.Equal(t, filepath.Join(dir, "codes.txt"), m.Update(enter))
	})

	t.Run("typing filters the entries", func(t *testing.T) {
		m := New(dir)
		m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})

		require.Len(t, m.visible, 1)
		assert.Equal(t, "codes.txt", m.visible[0].label)
	})
}
