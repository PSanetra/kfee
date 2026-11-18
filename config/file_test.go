package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveConfigDir(t *testing.T) {
	home := func() (string, error) { return filepath.FromSlash("/home/user"), nil }

	tests := []struct {
		name          string
		xdgConfigHome string
		expected      string
	}{
		{"uses XDG_CONFIG_HOME", "/xdg/config", "/xdg/config/ktea"},
		{"defaults to ~/.config", "", "/home/user/.config/ktea"},
		{"ignores relative XDG_CONFIG_HOME", "relative/config", "/home/user/.config/ktea"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, filepath.FromSlash(tt.expected), resolveConfigDir(filepath.FromSlash(tt.xdgConfigHome), home))
		})
	}
}

func TestConfigDirHonorsXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	assert.Equal(t, filepath.Join(dir, "ktea", "config.yaml"), Path())
	assert.Equal(t, filepath.Join(dir, "ktea", "connections.yaml"), ConnectionsPath())
	assert.Equal(t, filepath.Join(dir, "ktea", "themes"), ThemesDir())
}
