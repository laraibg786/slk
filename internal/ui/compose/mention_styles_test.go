package compose

import (
	"testing"

	"github.com/gammons/slk/internal/bubbles/mentionpicker"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui/styles"
)

// A theme change pushed through RefreshStyles must reach the mention
// picker popup, and a fresh composer must already be themed.
func TestRefreshStylesReachesMentionPicker(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", core.Theme{}) })
	styles.Apply("dark", core.Theme{})

	m := New("general")
	m.SetUsers([]mentionpicker.User{{ID: "U1", DisplayName: "alice", Username: "alice"}})
	m.mentionActive = true
	m.mentionPicker.Open()

	before := m.MentionPickerView(40)
	if before == "" {
		t.Fatal("expected a rendered popup")
	}

	styles.Apply("light", core.Theme{})
	if got := m.MentionPickerView(40); got != before {
		t.Fatal("popup changed before RefreshStyles pushed the new theme")
	}
	m.RefreshStyles()
	if got := m.MentionPickerView(40); got == before {
		t.Fatal("RefreshStyles did not reach the mention picker")
	}
}
