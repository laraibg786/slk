// Update runs per keystroke and View once per frame while the prompt is
// open. Both report allocations so the cost of moving or changing either
// path is visible.
//
// View is measured warm and cold. Warm is the steady state: the prompt
// is static while open, so every frame after the first is a cache hit.
// Cold is what a resize or a re-Open costs, and is dominated by lipgloss
// measuring and padding a bordered box -- not by anything this package
// does. Cold is unchanged from the pre-move implementation at 053d2a2.
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

// themedStyles mirrors what a host with a theme pushes in, including the
// BaseANSI fixup, which the defaults leave empty.
func themedStyles() Styles {
	s := DefaultStyles(true)
	s.BaseANSI = "\x1b[48;2;26;26;46m\x1b[38;2;224;224;224m"
	return s
}

func benchModel(body string) Model {
	m := New()
	m.SetStyles(themedStyles())
	m.SetSize(120, 40)
	m.Open("Delete message?", body, nil)
	return m
}

const benchBody = "the quick brown fox jumps over the lazy dog"

// The steady state: repeat frames while the prompt sits open.
func BenchmarkView(b *testing.B) {
	m := benchModel(benchBody)
	_ = m.View()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = m.View()
	}
}

// Alternating the body misses the single-entry cache every call, so this
// is the full render cost. Note that alternating the WIDTH does not: the
// box width is a clamped percentage, so nearby terminal widths collapse
// to the same render key.
func BenchmarkViewUncached(b *testing.B) {
	m := benchModel(benchBody)
	bodies := [2]string{benchBody, benchBody + "!"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		m.Open("Delete message?", bodies[i%2], nil)
		_ = m.View()
	}
}

// The truncation path adds a display-width measurement and a re-slice
// over a body that does not fit the box.
func BenchmarkViewUncachedTruncatedBody(b *testing.B) {
	long := strings.Repeat("überlange nachricht ", 40)
	m := benchModel(long)
	bodies := [2]string{long, long + "!"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		m.Open("Delete message?", bodies[i%2], nil)
		_ = m.View()
	}
}

// BaseANSI empty skips the reset fixup, which inflates the content the
// outer Render then has to measure. Isolates what the host's theme
// wiring costs.
func BenchmarkViewUncachedWithoutBaseANSI(b *testing.B) {
	m := New()
	m.SetSize(120, 40)
	bodies := [2]string{benchBody, benchBody + "!"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		m.Open("Delete message?", bodies[i%2], nil)
		_ = m.View()
	}
}

// Update copies the model, KeyMap included, on every key.
func BenchmarkUpdateConfirm(b *testing.B) {
	base := benchModel(benchBody)
	press := tea.KeyPressMsg{Code: 'y', Text: "y"}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m := base
		_, _ = m.Update(press)
	}
}

// The cancel path is what an unbound key takes, and is the common
// outcome for every key that is not a confirm.
func BenchmarkUpdateCancel(b *testing.B) {
	base := benchModel(benchBody)
	press := tea.KeyPressMsg{Code: tea.KeyEscape}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m := base
		_, _ = m.Update(press)
	}
}

// A non-key message reaches Update on every tick the host forwards.
func BenchmarkUpdateIgnoredMsg(b *testing.B) {
	m := benchModel(benchBody)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = m.Update(sentinelMsg{})
	}
}
