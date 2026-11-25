package keymap

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads the key bindings from a keymap.yaml file: action names mapped to lists of keys.
// A missing or empty file returns no overrides, so the defaults stay.
func Load(path string) (map[string][]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var overrides map[string][]string
	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&overrides); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return overrides, nil
}
