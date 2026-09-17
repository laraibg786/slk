package confirmprompt

import "charm.land/bubbles/v2/key"

// KeyMap is the prompt's key bindings. Keys outside Confirm cancel, so
// Cancel's bindings drive the footer's help text rather than the match.
type KeyMap struct {
	Confirm key.Binding
	Cancel  key.Binding
}

// DefaultKeyMap returns the standard bindings: y/Y/Enter confirm,
// n/N/Esc cancel.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Confirm: key.NewBinding(
			key.WithKeys("y", "Y", "enter"),
			key.WithHelp("y", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("n", "N", "esc"),
			key.WithHelp("n/Esc", "cancel"),
		),
	}
}
