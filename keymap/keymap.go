// Package keymap holds the key bindings of kfee. Every binding is an action with default keys,
// the keys can be replaced per action in keymap.yaml.
package keymap

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// Action names a behaviour that a key triggers, e.g. moving the cursor of a table.
type Action string

const (
	TableUp       Action = "table.up"
	TableDown     Action = "table.down"
	TablePageUp   Action = "table.page_up"
	TablePageDown Action = "table.page_down"
	TableTop      Action = "table.top"
	TableBottom   Action = "table.bottom"
	TableHalfUp   Action = "table.half_page_up"
	TableHalfDown Action = "table.half_page_down"
)

// Actions of the tabs, active on every page.
var (
	TabsPrevious = Define("tabs.previous", "global", "ctrl+left")
	TabsNext     = Define("tabs.next", "global", "ctrl+right")
)

// Actions of the records list of a topic.
var (
	RecordsDecoders       = Define("records.decoders", "records", "d")
	RecordsSearch         = Define("records.search", "records", "/")
	RecordsSearchExtended = Define("records.search_extended", "records", "ctrl+f")
	RecordsToggleOrig     = Define("records.toggle_original", "records", "t")
	RecordsCopy           = Define("records.copy", "records", "c")
	RecordsEndConsume     = Define("records.end_consumption", "records", "f2")
	RecordsOpen           = Define("records.open", "records", "enter")
)

// definition is an action with its default keys and the scope in which its keys must not clash.
type definition struct {
	defaults []string
	scope    string
}

var definitions = map[Action]definition{
	TableUp:       {[]string{"up"}, "table"},
	TableDown:     {[]string{"down"}, "table"},
	TablePageUp:   {[]string{"pgup"}, "table"},
	TablePageDown: {[]string{"pgdown"}, "table"},
	TableTop:      {[]string{"home"}, "table"},
	TableBottom:   {[]string{"end"}, "table"},
	TableHalfUp:   {nil, "table"},
	TableHalfDown: {nil, "table"},
}

// Define registers an action with its default keys. Keys of actions in the same scope must not clash.
// The pages of kfee call it when their package is initialised, e.g.
//
//	var actionCopy = keymap.Define("records.copy", "records", "c")
//
// It panics on a duplicate name, which is a programming error.
func Define(a Action, scope string, defaults ...string) Action {
	if _, ok := definitions[a]; ok {
		panic(fmt.Sprintf("keymap: action %q is defined twice", a))
	}
	definitions[a] = definition{defaults: defaults, scope: scope}
	return a
}

// reserved keys cannot be bound to an action, they stay with the framework.
var reserved = map[string]bool{
	"ctrl+c": true,
}

var (
	mu sync.RWMutex
	// active holds the keys in use, nil until the first use so that all actions are defined by then.
	active map[Action][]string
)

func defaultKeys() map[Action][]string {
	keys := make(map[Action][]string, len(definitions))
	for a, d := range definitions {
		keys[a] = append([]string(nil), d.defaults...)
	}
	return keys
}

// current returns the active keys, the caller must hold mu.
func current() map[Action][]string {
	if active == nil {
		active = defaultKeys()
	}
	return active
}

// Reset restores the default keys of all actions.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	active = nil
}

// Apply installs the keys from keymap.yaml. Actions that are not in overrides keep their defaults.
// An empty list unbinds the action. Nothing is changed when an override is invalid.
func Apply(overrides map[string][]string) error {
	next := defaultKeys()
	changed := map[Action]bool{}
	for name, keys := range overrides {
		a := Action(name)
		if _, ok := definitions[a]; !ok {
			return fmt.Errorf("keymap: unknown action %q", name)
		}
		for _, k := range keys {
			if strings.TrimSpace(k) == "" || strings.ContainsAny(k, " \t") {
				return fmt.Errorf("keymap: invalid key %q for %s", k, name)
			}
			if reserved[k] {
				return fmt.Errorf("keymap: key %q cannot be bound (%s)", k, name)
			}
		}
		next[a] = append([]string(nil), keys...)
		// an override that repeats the defaults is not a change, so a copy of the defaults loads
		if !slices.Equal(keys, definitions[a].defaults) {
			changed[a] = true
		}
	}
	if err := checkClashes(next, changed); err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	active = next
	return nil
}

// checkClashes rejects a key of an overridden action that another action it can be active with also binds.
// Default keys may overlap, pages use the same key for different actions in different modes.
func checkClashes(keys map[Action][]string, changed map[Action]bool) error {
	for a := range changed {
		scope := definitions[a].scope
		for _, k := range keys[a] {
			for b, bks := range keys {
				if b != a && overlaps(scope, definitions[b].scope) && slices.Contains(bks, k) {
					return fmt.Errorf("keymap: key %q is bound to both %s and %s", k, a, b)
				}
			}
		}
	}
	return nil
}

// overlaps reports whether actions of the two scopes can be active at the same time. Table and global
// actions are active on every page, so they overlap with any scope.
func overlaps(a, b string) bool {
	return a == b || a == "table" || b == "table" || a == "global" || b == "global"
}

// Matches reports whether the key press triggers the action.
func Matches(msg tea.KeyPressMsg, a Action) bool {
	mu.Lock()
	defer mu.Unlock()
	for _, k := range current()[a] {
		if msg.String() == k {
			return true
		}
	}
	return false
}

// Keys returns the keys currently bound to the action.
func Keys(a Action) []string {
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), current()[a]...)
}

var displayNames = map[string]string{
	"left":  "←",
	"right": "→",
	"up":    "↑",
	"down":  "↓",
}

// Hint formats the keys of the action for the status bar and the help, e.g. "C-s", "F2" or "↑/↓".
// It is empty when the action is unbound.
func Hint(a Action) string {
	keys := Keys(a)
	shown := make([]string, 0, len(keys))
	for _, k := range keys {
		shown = append(shown, display(k))
	}
	return strings.Join(shown, "/")
}

func display(k string) string {
	if name, ok := displayNames[k]; ok {
		return name
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return "C-" + rest
	}
	if strings.HasPrefix(k, "f") && len(k) > 1 && k[1] >= '0' && k[1] <= '9' {
		return "F" + k[1:]
	}
	return k
}
