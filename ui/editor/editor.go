// Package editor opens text in the editor of the user, $EDITOR, or nano when it is not set.
package editor

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Edit opens the text in the editor. The file the editor leaves behind is passed to done, with its
// text, or with the error when the editor or the file failed. The message done returns is delivered
// to the program. An error is returned when the file cannot be written, and the editor is not opened.
func Edit(text string, done func(text string, err error) tea.Msg) (tea.Cmd, error) {
	file, err := os.CreateTemp("", "kfee-*.txt")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}

	name, args := command()
	cmd := exec.Command(name, append(args, path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return done("", err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return done("", err)
		}
		return done(strings.TrimSuffix(string(content), "\n"), nil)
	}), nil
}

// command is the editor from $EDITOR, nano when it is not set.
func command() (string, []string) {
	fields := strings.Fields(os.Getenv("EDITOR"))
	if len(fields) > 0 {
		return fields[0], fields[1:]
	}
	return "nano", nil
}
