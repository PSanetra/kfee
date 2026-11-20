package record_details_page

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordDetailsPageAttachFiles(t *testing.T) {
	m, _, _ := newGenerationPage(t, nil, "r1")
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	require.NoError(t, os.WriteFile(first, []byte("1 = NEW"), 0o644))
	require.NoError(t, os.WriteFile(second, []byte("2 = DONE"), 0o644))
	ctrl := func(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}

	m.Update(tea.KeyPressMsg{Code: 'e'})
	m.transformEditor.attach(first)

	// the picker starts in the directory of the last attachment, the first entry is ".."
	m.Update(ctrl('o'))
	require.NotNil(t, m.transformEditor.files)
	assert.Equal(t, dir, m.transformEditor.files.Dir())
	m.Update(down)
	m.Update(down)
	m.Update(enter)
	assert.Nil(t, m.transformEditor.files)
	assert.Equal(t, []string{first, second}, m.transformEditor.attachments)

	m.Update(ctrl('r'))
	assert.Equal(t, []string{first}, m.transformEditor.attachments)

	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Nil(t, m.transformEditor)
}
