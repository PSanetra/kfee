package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveCacheDir(t *testing.T) {
	home := func() (string, error) { return filepath.FromSlash("/home/user"), nil }

	assert.Equal(t, filepath.FromSlash("/xdg/cache/kfee/schemas"), resolveCacheDir(filepath.FromSlash("/xdg/cache"), home))
	assert.Equal(t, filepath.FromSlash("/home/user/.cache/kfee/schemas"), resolveCacheDir("", home))
	assert.Equal(t, filepath.FromSlash("/home/user/.cache/kfee/schemas"), resolveCacheDir("relative/cache", home))
}
