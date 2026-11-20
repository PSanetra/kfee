package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestsNeverUseTheRealHome(t *testing.T) {
	tmp, err := filepath.Abs(os.TempDir())
	require.NoError(t, err)

	paths := map[string]string{
		"config":          Path(),
		"connections":     ConnectionsPath(),
		"themes":          ThemesDir(),
		"transformations": TransformationsDir(),
		"schema cache":    SchemaCacheDir(),
	}
	for name, path := range paths {
		assert.True(t, strings.HasPrefix(path, tmp+string(filepath.Separator)), "%s resolves to %s, outside the temporary directory", name, path)
		if originalHome != "" {
			assert.False(t, strings.HasPrefix(path, originalHome+string(filepath.Separator)), "%s resolves into the real home directory: %s", name, path)
		}
	}
}
