package keymap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeKeymap(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keymap.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadMissingFileKeepsDefaults(t *testing.T) {
	overrides, err := Load(filepath.Join(t.TempDir(), "keymap.yaml"))
	require.NoError(t, err)
	assert.Nil(t, overrides)
}

func TestLoadEmptyFileKeepsDefaults(t *testing.T) {
	overrides, err := Load(writeKeymap(t, ""))
	require.NoError(t, err)
	assert.Nil(t, overrides)
}

func TestLoadAppliesOverrides(t *testing.T) {
	t.Cleanup(Reset)
	path := writeKeymap(t, `# my keys
records.decoders: [x, ctrl+s]
table.page_down: [pgdown, ctrl+v]
`)

	overrides, err := Load(path)
	require.NoError(t, err)
	require.NoError(t, Apply(overrides))

	assert.Equal(t, []string{"x", "ctrl+s"}, Keys(RecordsDecoders))
	assert.Equal(t, []string{"pgdown", "ctrl+v"}, Keys(TablePageDown))
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	_, err := Load(writeKeymap(t, "records.decoders: [x\n"))
	assert.ErrorContains(t, err, "keymap.yaml")
}

func TestLoadUnknownActionIsRejectedByApply(t *testing.T) {
	t.Cleanup(Reset)
	overrides, err := Load(writeKeymap(t, "records.nope: [x]\n"))
	require.NoError(t, err)
	assert.ErrorContains(t, Apply(overrides), `unknown action "records.nope"`)
}
