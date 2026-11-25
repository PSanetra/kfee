package keymap

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// WriteDefaults writes the default keys of all actions in the format of keymap.yaml, grouped by scope.
// The output is a valid keymap.yaml itself, so lines can be copied and changed.
func WriteDefaults(w io.Writer) error {
	byScope := map[string][]Action{}
	for a, d := range definitions {
		byScope[d.scope] = append(byScope[d.scope], a)
	}

	var b strings.Builder
	b.WriteString("# Default keys of kfee. Copy the lines to change into keymap.yaml.\n")
	b.WriteString("# A list replaces the default keys, [] unbinds the action.\n")
	for _, scope := range slices.Sorted(maps.Keys(byScope)) {
		actions := byScope[scope]
		slices.Sort(actions)
		fmt.Fprintf(&b, "\n# %s\n", scope)
		for _, a := range actions {
			fmt.Fprintf(&b, "%s: [%s]\n", a, strings.Join(definitions[a].defaults, ", "))
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}
