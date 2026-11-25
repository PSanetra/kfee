package cmdbar

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"kfee/keymap"
	"kfee/kontext"
	"kfee/styles"
	"kfee/ui"
	"kfee/ui/components/statusbar"
)

type DeleteMsgFn[T any] func(T) string

type DeleteFn[T any] func(T) tea.Cmd

type ValidateFn[T any] func(T) (bool, tea.Cmd)

type DeleteCmdBar[T any] struct {
	active        bool
	deleteConfirm *huh.Confirm
	msg           string
	deleteValue   T
	deleteMsgFunc DeleteMsgFn[T]
	deleteFunc    DeleteFn[T]
	validateFn    ValidateFn[T]
	deleteAction  keymap.Action
}

type Option[T any] func(bar *DeleteCmdBar[T])

// WithDeleteAction sets the action that toggles the delete bar. The caller must trigger the same action
// to open it, so a rebinding in keymap.yaml applies to both. By default it is the table's delete action.
func WithDeleteAction[T any](a keymap.Action) Option[T] {
	return func(bar *DeleteCmdBar[T]) {
		bar.deleteAction = a
	}
}

func WithValidateFn[T any](vFn ValidateFn[T]) Option[T] {
	return func(bar *DeleteCmdBar[T]) {
		bar.validateFn = vFn
	}
}

func (s *DeleteCmdBar[any]) View(ktx *kontext.ProgramKtx, renderer *ui.Renderer) string {
	s.deleteConfirm.Title(lipgloss.NewStyle().
		Foreground(styles.Current.Text).
		Render(lipgloss.NewStyle().MarginRight(2).Render("🗑️  " + s.deleteMsgFunc(s.deleteValue)))).
		WithButtonAlignment(lipgloss.Left).
		Focus()

	s.deleteConfirm.WithTheme(styles.Huh)

	return renderer.RenderWithStyle(s.deleteConfirm.View(), styles.CmdBarWithWidth(ktx.WindowWidth-BorderedPadding))
}

func (s *DeleteCmdBar[any]) IsFocussed() bool {
	return s.active
}

func (s *DeleteCmdBar[any]) Shortcuts() []statusbar.Shortcut {
	if s.active {
		return []statusbar.Shortcut{
			{"Confirm", keymap.Hint(actionConfirm)},
			{"Select Cancel", "c"},
			{"Select Delete", "d"},
			{"Cancel", fmt.Sprintf("%s/%s", keymap.Hint(actionCancel), keymap.Hint(s.deleteAction))},
		}
	}
	return nil
}

func (s *DeleteCmdBar[any]) Update(msg tea.Msg) (bool, tea.Msg, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case keymap.Matches(msg, s.deleteAction):
			s.active = !s.active
			return s.active, nil, nil
		case keymap.Matches(msg, actionCancel):
			s.active = false
			return s.active, nil, nil
		case keymap.Matches(msg, actionConfirm):
			if s.validateFn != nil {
				valid, cmd := s.validateFn(s.deleteValue)
				if !valid {
					return s.active, nil, cmd
				}
			}
			if s.deleteConfirm.GetValue().(bool) {
				s.deleteConfirm = newDeleteConfirm()
				return s.active, nil, s.deleteFunc(s.deleteValue)
			} else {
				s.active = false
				return s.active, nil, nil
			}
		}
	}
	confirm, cmd := s.deleteConfirm.Update(msg)
	if cmd != nil {
		// if msg has been handled do not propagate it
		msg = nil
	}
	if c, ok := confirm.(*huh.Confirm); ok {
		s.deleteConfirm = c
	}

	return s.active, msg, nil
}

func (s *DeleteCmdBar[T]) Delete(d T) {
	s.deleteValue = d
}

func (s *DeleteCmdBar[any]) Hide() {
	s.active = false
}

func newDeleteConfirm() *huh.Confirm {
	return huh.NewConfirm().
		Inline(true).
		Affirmative("Delete!").
		Negative("Cancel.").
		WithKeyMap(&huh.KeyMap{
			Confirm: huh.ConfirmKeyMap{
				Submit: key.NewBinding(key.WithKeys("enter")),
				Toggle: key.NewBinding(key.WithKeys("h", "l", "right", "left")),
				Accept: key.NewBinding(key.WithKeys("d")),
				Reject: key.NewBinding(key.WithKeys("c")),
			},
		}).(*huh.Confirm)
}

func NewDeleteCmdBar[T any](
	deleteMsgFunc DeleteMsgFn[T],
	deleteFunc DeleteFn[T],
	options ...Option[T],
) *DeleteCmdBar[T] {
	bar := DeleteCmdBar[T]{
		deleteFunc:    deleteFunc,
		deleteMsgFunc: deleteMsgFunc,
		deleteConfirm: newDeleteConfirm(),
		deleteAction:  actionDelete,
	}

	for _, o := range options {
		o(&bar)
	}

	return &bar
}
