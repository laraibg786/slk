// Update runs per keystroke and View once per frame while the prompt is
// open. Both report allocations so the cost of moving or changing either
// path is visible.
//
// View's cost is lipgloss rendering a bordered, padded box, and the model
// holds no render cache, so every frame the prompt is open pays it in
// full. Measured against the pre-move implementation at 053d2a2 it is
// unchanged in both time and allocations.
//
// Key handling is slower in relative terms than the string switch it
// replaced: key.Matches formats the key name, where the caller used to
// pass an already-normalised string. At one call per keystroke that is
// not worth optimising, and it is noted here so the ratio is not read as
// a defect.
package confirmprompt

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// View runs once per frame for as long as the prompt is open, and the
// model holds no render cache, so this is the whole per-frame cost.
func BenchmarkView(b *testing.B) {
	m := New()
	m.Styles.BaseANSI = "\x1b[48;2;26;26;46m\x1b[38;2;224;224;224m"
	m.SetSize(120, 40)
	m.Open("Delete message?", "the quick brown fox jumps over the lazy dog", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = m.View()
	}
}

// The truncation path adds a display-width measurement and a re-slice
// over a body that does not fit the box.
func BenchmarkViewTruncatedBody(b *testing.B) {
	m := New()
	m.Styles.BaseANSI = "\x1b[48;2;26;26;46m\x1b[38;2;224;224;224m"
	m.SetSize(120, 40)
	m.Open("Delete message?", strings.Repeat("überlange nachricht ", 40), nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = m.View()
	}
}

// BaseANSI empty skips the reset fixup, which is two passes over the
// composed content. Measures what the host's theme wiring costs.
func BenchmarkViewWithoutBaseANSI(b *testing.B) {
	m := New()
	m.SetSize(120, 40)
	m.Open("Delete message?", "the quick brown fox jumps over the lazy dog", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = m.View()
	}
}

// Update copies the model, Styles and KeyMap included, on every key.
func BenchmarkUpdateConfirm(b *testing.B) {
	base := New()
	base.SetSize(120, 40)
	press := tea.KeyPressMsg{Code: 'y', Text: "y"}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m := base
		m.Open("Delete message?", "body", nil)
		_, _ = m.Update(press)
	}
}

// The cancel path is what an unbound key takes, and is the common
// outcome for every key that is not a confirm.
func BenchmarkUpdateCancel(b *testing.B) {
	base := New()
	base.SetSize(120, 40)
	press := tea.KeyPressMsg{Code: tea.KeyEscape}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m := base
		m.Open("Delete message?", "body", nil)
		_, _ = m.Update(press)
	}
}

// A non-key message reaches Update on every tick the host forwards.
func BenchmarkUpdateIgnoredMsg(b *testing.B) {
	m := New()
	m.SetSize(120, 40)
	m.Open("Delete message?", "body", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = m.Update(sentinelMsg{})
	}
}
