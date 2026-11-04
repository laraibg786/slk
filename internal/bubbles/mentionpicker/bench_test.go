// View runs once per frame while the picker is open; SetQuery runs once per
// keystroke after the "@" and filters and ranks every candidate.
//
// View has no cache: each frame rebuilds every visible row (label format,
// style render, suffix) and the popup. It only ever draws the filtered list,
// which filter() caps at MaxVisible, so View cost does not depend on the
// number of users; the scaling benchmarks show that. "warm" repeats the same
// state; "cold" moves the selected row each frame, which changes which row
// takes the selected style.
//
// SetQuery is where the cost scales: for every user it folds the display name
// and the username (allocating a string when the name has upper case or
// accents), scans word boundaries, may squash the name, and appends to the tier
// slices; the matches are then sorted. The few/many/none queries vary how many
// candidates survive into the sort. SetUsers only stores the slice;
// MoveUp/MoveDown and Select are pure index work and are pinned at their
// allocation counts.
package mentionpicker

import (
	"fmt"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/bubbles/chrome"
)

// benchSink keeps results alive so the compiler cannot drop the calls.
var (
	benchSink       string
	benchSelectSink *MentionResult
)

// benchStyles mirrors a themed host: colours and a background on every
// style, and BaseANSI set, so the benchmarks pay for real SGR output.
func benchStyles() Styles {
	bg := lipgloss.Color("#1a1b26")
	fg := lipgloss.Color("#c0caf5")
	accent := lipgloss.Color("#7aa2f7")
	muted := lipgloss.Color("#565f89")
	return Styles{
		Chrome: chrome.Styles{
			Box:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).BorderBackground(bg).Background(bg).Foreground(fg).Padding(1, 1),
			Title:       lipgloss.NewStyle().Foreground(accent).Background(bg).Bold(true),
			Row:         lipgloss.NewStyle().Foreground(fg).Background(bg),
			RowSelected: lipgloss.NewStyle().Foreground(bg).Background(accent),
			Marker:      lipgloss.NewStyle().Foreground(accent).Background(bg),
			Empty:       lipgloss.NewStyle().Foreground(muted).Background(bg),
			Footer:      lipgloss.NewStyle().Foreground(muted).Background(bg),
			Thumb:       lipgloss.NewStyle().Foreground(accent).Background(bg),
			Track:       lipgloss.NewStyle().Foreground(muted).Background(bg),
			BaseANSI:    "\x1b[48;2;26;27;38m",
		},
		Name:  lipgloss.NewStyle().Foreground(fg).Background(bg),
		Muted: lipgloss.NewStyle().Foreground(muted).Background(bg),
	}
}

// benchUsers builds n synthetic users. Display names and handles carry
// separators so the word and squashed match paths run; every third user is
// outside the channel and every seventh is external.
func benchUsers(n int) []User {
	users := make([]User, n)
	for i := range users {
		users[i] = User{
			ID:          fmt.Sprintf("U%05d", i),
			DisplayName: fmt.Sprintf("User-%03d Example", i),
			Username:    fmt.Sprintf("member_%03d", i),
			InChannel:   i%3 != 0,
			IsExternal:  i%7 == 0,
		}
	}
	return users
}

// benchQueries: "many" matches every user's display name by prefix, "few"
// matches about two percent (a prefix of the number), "word" hits the second
// word of every name, "squash" only matches once separators are removed, and
// "none" matches nobody.
var benchQueries = []struct{ name, q string }{
	{"many", "user"},
	{"few", "user-04"},
	{"word", "exam"},
	{"squash", "user04"},
	{"none", "zzz"},
}

var benchSizes = []int{5, 50, 500}

func benchModel(users []User, query string) *Model {
	m := New(WithStyles(benchStyles()), WithWidth(60))
	m.SetUsers(users)
	m.Open()
	m.SetQuery(query)
	return &m
}

func BenchmarkViewWarm(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("users=%d", n), func(b *testing.B) {
			m := benchModel(benchUsers(n), "user")
			b.ReportAllocs()
			for b.Loop() {
				benchSink = m.View()
			}
		})
	}
}

func BenchmarkViewCold(b *testing.B) {
	m := benchModel(benchUsers(500), "user")
	rows := len(m.Filtered())
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		m.selected = i % rows
		i++
		benchSink = m.View()
	}
}

// BenchmarkViewFewMatches draws a list shorter than MaxVisible.
func BenchmarkViewFewMatches(b *testing.B) {
	m := benchModel(benchUsers(500), "user-049")
	b.ReportAllocs()
	for b.Loop() {
		benchSink = m.View()
	}
}

func BenchmarkSetQuery(b *testing.B) {
	for _, n := range benchSizes {
		for _, q := range benchQueries {
			b.Run(fmt.Sprintf("users=%d/%s", n, q.name), func(b *testing.B) {
				m := benchModel(benchUsers(n), "")
				b.ReportAllocs()
				for b.Loop() {
					m.SetQuery(q.q)
				}
				benchSink = m.Query()
			})
		}
	}
}

// BenchmarkSetQueryEmpty is the "@" keystroke: an empty query ranks everyone.
func BenchmarkSetQueryEmpty(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("users=%d", n), func(b *testing.B) {
			m := benchModel(benchUsers(n), "")
			b.ReportAllocs()
			for b.Loop() {
				m.SetQuery("")
			}
		})
	}
}

func BenchmarkSetUsers(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("users=%d", n), func(b *testing.B) {
			users := benchUsers(n)
			m := New()
			b.ReportAllocs()
			for b.Loop() {
				m.SetUsers(users)
			}
		})
	}
}

func BenchmarkMove(b *testing.B) {
	m := benchModel(benchUsers(500), "user")
	b.ReportAllocs()
	for b.Loop() {
		m.MoveDown()
		m.MoveUp()
	}
}

func BenchmarkSelect(b *testing.B) {
	m := benchModel(benchUsers(500), "user")
	m.MoveDown()
	b.ReportAllocs()
	for b.Loop() {
		benchSelectSink = m.Select()
	}
}
