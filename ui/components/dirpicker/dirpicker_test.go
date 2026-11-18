package dirpicker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tree(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if strings.HasSuffix(f, "/") {
			require.NoError(t, os.MkdirAll(path, 0o755))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, nil, 0o644))
	}
	return dir
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func labels(m *Model) []string {
	var l []string
	for _, e := range m.visible {
		l = append(l, e.label)
	}
	return l
}

func TestDirPicker(t *testing.T) {
	t.Run("lists subdirectories without hidden ones", func(t *testing.T) {
		dir := tree(t, "b/x.proto", "a/", ".git/y.proto", "file.proto")

		m := New(dir, ".proto", nil)

		assert.Equal(t, []string{"..", "a/", "b/"}, labels(m))
		assert.Equal(t, "a/", m.visible[m.cursor].label, "starts on the first subdirectory")
	})

	t.Run("counts files recursively, skipping hidden and node_modules directories", func(t *testing.T) {
		dir := tree(t, "a/x.proto", "a/b/y.proto", "a/.cache/z.proto", "a/node_modules/n.proto", "a/readme.md")

		m := New(dir, ".proto", nil)

		assert.Equal(t, count{n: 2}, m.countOf(filepath.Join(dir, "a")))
		assert.Contains(t, ansi.Strip(m.View(80)), "2 .proto")
	})

	t.Run("filters, opens and goes up", func(t *testing.T) {
		dir := tree(t, "protos/acme/", "docs/")
		m := New(dir, ".proto", nil)

		m.Update(key("p"))
		m.Update(key("r"))
		assert.Equal(t, []string{"protos/"}, labels(m))
		m.Update(key("enter"))
		assert.Equal(t, filepath.Join(dir, "protos"), m.Dir())
		assert.False(t, m.Filtering(), "opening clears the filter")

		m.Update(key("backspace"))
		assert.Equal(t, dir, m.Dir())
		assert.Equal(t, "protos/", m.visible[m.cursor].label, "the directory we came from is selected")
	})

	t.Run("backspace edits the filter first", func(t *testing.T) {
		dir := tree(t, "protos/")
		m := New(dir, ".proto", nil)
		m.Update(key("x"))

		m.Update(key("backspace"))

		assert.Equal(t, dir, m.Dir())
		assert.False(t, m.Filtering())
	})

	t.Run("ctrl+s chooses the current directory", func(t *testing.T) {
		m := New(tree(t, "a/"), ".proto", nil)

		assert.False(t, m.Update(key("down")))
		assert.True(t, m.Update(key("ctrl+s")))
	})

	t.Run("offers recent directories", func(t *testing.T) {
		dir := tree(t, "a/")
		recent := tree(t, "x/")

		m := New(dir, ".proto", []string{recent, filepath.Join(recent, "missing"), recent})

		require.Len(t, m.visible, 3)
		assert.Equal(t, recentEntry, m.visible[0].kind)
		assert.Equal(t, recent, m.visible[0].path)
		m.cursor = 0
		m.Update(key("enter"))
		assert.Equal(t, recent, m.Dir())
	})

	t.Run("starts at the closest existing parent", func(t *testing.T) {
		dir := tree(t, "a/")

		m := New(filepath.Join(dir, "a", "missing", "deeper"), ".proto", nil)

		assert.Equal(t, filepath.Join(dir, "a"), m.Dir())
	})

	t.Run("marks truncated counts", func(t *testing.T) {
		var files []string
		for i := 0; i < countBudget+10; i++ {
			files = append(files, filepath.Join("big", "f"+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676))+".proto"))
		}
		dir := tree(t, files...)

		m := New(dir, ".proto", nil)

		assert.True(t, m.countOf(filepath.Join(dir, "big")).truncated)
		assert.Contains(t, ansi.Strip(m.View(80)), "+ .proto")
	})
}
