package confirmprompt

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update handles one key press. Anything that is not a Confirm binding
// cancels. The prompt closes itself either way, so callers detect the
// outcome with IsVisible.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.visible {
		return m, nil
	}

	var cmd tea.Cmd
	if m.confirms(press) && m.onConfirm != nil {
		fn := m.onConfirm
		cmd = func() tea.Msg { return fn() }
	}
	m.Close()
	return m, cmd
}

// confirms reports whether press matches the Confirm binding, ignoring
// modifiers on non-printable keys: a stray shift on Enter must not turn
// a confirm into a cancel.
func (m Model) confirms(press tea.KeyPressMsg) bool {
	if key.Matches(press, m.KeyMap.Confirm) {
		return true
	}
	if press.Text != "" || press.Mod == 0 {
		return false
	}
	press.Mod = 0
	return key.Matches(press, m.KeyMap.Confirm)
}
