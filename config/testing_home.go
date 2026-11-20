package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// originalHome is the home directory of the user running the tests, tests must never use it.
var originalHome = os.Getenv("HOME")

// init isolates test binaries from the configuration, data and cache of the user. The XDG
// directories point to a fresh temporary directory, so every path resolved by kfee lands there.
// HOME is left alone for other tools. Packages importing config get this guard.
func init() {
	if !testing.Testing() {
		return
	}
	root, err := os.MkdirTemp("", "kfee-test-")
	if err != nil {
		panic(fmt.Sprintf("tests need a temporary directory: %v", err))
	}
	for name, dir := range map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
	} {
		if err := os.Setenv(name, dir); err != nil {
			panic(fmt.Sprintf("tests cannot set %s: %v", name, err))
		}
	}
}
