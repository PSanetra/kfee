package keymap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDefaultsListsActionsByScope(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, WriteDefaults(&out))
	text := out.String()

	assert.Contains(t, text, "\n# table\n")
	assert.Contains(t, text, "table.up: [up]\n")
	assert.Contains(t, text, "records.decoders: [d]\n")
	assert.Contains(t, text, "table.half_page_down: []\n", "unbound actions are shown with an empty list")
}

func TestWriteDefaultsLoadsAsKeymapFile(t *testing.T) {
	t.Cleanup(Reset)

	var out bytes.Buffer
	require.NoError(t, WriteDefaults(&out))
	path := filepath.Join(t.TempDir(), "keymap.yaml")
	require.NoError(t, os.WriteFile(path, out.Bytes(), 0o600))

	overrides, err := Load(path)
	require.NoError(t, err)
	require.NoError(t, Apply(overrides), "a copy of the defaults must not clash with itself")

	assert.Equal(t, []string{"d"}, Keys(RecordsDecoders))
	assert.Equal(t, []string{"enter"}, Keys(RecordsOpen))
	assert.True(t, strings.Contains(out.String(), "# Default keys"))
}

func TestOverrideRepeatingDefaultIsNoChange(t *testing.T) {
	t.Cleanup(Reset)
	// repeating the default is not a change, so it is not checked against the other actions
	require.NoError(t, Apply(map[string][]string{"records.open": {"enter"}}))
}
