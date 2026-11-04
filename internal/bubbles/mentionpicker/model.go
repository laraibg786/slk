package mentionpicker

import (
	"fmt"
	"sort"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/text"
)

var _ chrome.Dialog = (*Model)(nil)

const MaxVisible = 7

type User struct {
	ID          string
	DisplayName string
	Username    string
	// InChannel indicates whether the user is a member of the active
	// channel. False sorts the user into the not-in-channel tier (below
	// in-channel users in the picker). The caller is responsible for
	// populating this field. Special mentions (@here / @channel /
	// @everyone) always have InChannel=true. For workspace users, the
	// compose layer computes the value from the active channel's member
	// set (see compose.rebuildMentionUsers); when no membership data
	// has been loaded yet, compose defaults it to true to preserve the
	// pre-channel-aware behavior.
	InChannel bool
	// IsExternal indicates a Slack Connect / shared-channel guest
	// whose home team_id differs from the workspace TeamID. Drives
	// the "(ext)" suffix in the picker.
	IsExternal bool
}

type MentionResult struct {
	UserID      string
	DisplayName string
}

var specialMentions = []User{
	{ID: "special:here", DisplayName: "here", Username: "here", InChannel: true},
	{ID: "special:channel", DisplayName: "channel", Username: "channel", InChannel: true},
	{ID: "special:everyone", DisplayName: "everyone", Username: "everyone", InChannel: true},
}

// Styles is everything the picker draws with. The composer maps the theme
// onto it and pushes it at construction and on every theme change.
type Styles struct {
	// Chrome is the shared popup styling (box, rows, marker).
	Chrome chrome.Styles
	// Name styles a row's label; the selected row is also bold.
	Name lipgloss.Style
	// Muted styles the label of a not-in-channel row at rest and every
	// row's "(not in channel)" / "(ext)" suffix.
	Muted lipgloss.Style
}

// Option configures a Model at construction.
type Option func(*Model)

// WithStyles sets the initial styles.
func WithStyles(s Styles) Option { return func(m *Model) { m.styles = s } }

// WithWidth sets the popup width.
func WithWidth(w int) Option { return func(m *Model) { m.width = w } }

type Model struct {
	users    []User
	filtered []User
	query    string
	selected int
	visible  bool
	styles   Styles
	width    int
}

func New(opts ...Option) Model {
	m := Model{}
	for _, o := range opts {
		o(&m)
	}
	return m
}

// SetStyles replaces the picker's styles.
func (m *Model) SetStyles(s Styles) { m.styles = s }

// SetWidth sets the popup width the composer allots.
func (m *Model) SetWidth(w int) { m.width = w }

func (m *Model) SetUsers(users []User) {
	m.users = users
}

// Users returns the user list most recently set via SetUsers.
// Used by tests; not part of the picker's input API.
func (m *Model) Users() []User {
	return m.users
}

func (m *Model) Open() {
	m.visible = true
	m.query = ""
	m.selected = 0
	m.filter()
}

func (m *Model) Close() {
	m.visible = false
	m.query = ""
	m.selected = 0
	m.filtered = nil
}

func (m *Model) IsVisible() bool {
	return m.visible
}

func (m *Model) SetQuery(q string) {
	m.query = q
	m.selected = 0
	m.filter()
}

func (m *Model) Query() string {
	return m.query
}

func (m *Model) Filtered() []User {
	return m.filtered
}

func (m *Model) Selected() int {
	return m.selected
}

func (m *Model) MoveUp() {
	if m.selected > 0 {
		m.selected--
	}
}

func (m *Model) MoveDown() {
	if m.selected < len(m.filtered)-1 {
		m.selected++
	}
}

func (m *Model) Select() *MentionResult {
	if len(m.filtered) == 0 {
		return nil
	}
	if m.selected < 0 || m.selected >= len(m.filtered) {
		return nil
	}
	u := m.filtered[m.selected]
	return &MentionResult{
		UserID:      u.ID,
		DisplayName: u.DisplayName,
	}
}

// rankedUser pairs a candidate with its match rank so the sort can put
// stronger matches first within a membership tier.
// It holds the index into the user list rather than a copy of the user, so
// ranking 500 candidates moves 16-byte entries, not 500 User structs.
type rankedUser struct {
	idx  int
	rank matchRank
}

// sortRanked orders candidates by match rank, then display name, then ID.
// sort.Slice is unstable, so the ID tie-break keeps the dropdown from
// reshuffling between identical keystrokes when two entries share a
// display name.
func sortRanked(users []User, s []rankedUser) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].rank != s[j].rank {
			return s[i].rank < s[j].rank
		}
		a, b := users[s[i].idx], users[s[j].idx]
		if a.DisplayName != b.DisplayName {
			return a.DisplayName < b.DisplayName
		}
		return a.ID < b.ID
	})
}

func (m *Model) filter() {
	q := text.Fold(m.query)
	sq := squash(q)

	var specials []User
	var inCh, notInCh []rankedUser
	for _, u := range specialMentions {
		if rankUser(u, q, sq) != rankNone {
			specials = append(specials, u)
		}
	}
	for i, u := range m.users {
		r := rankUser(u, q, sq)
		if r == rankNone {
			continue
		}
		if u.InChannel {
			inCh = append(inCh, rankedUser{idx: i, rank: r})
		} else {
			notInCh = append(notInCh, rankedUser{idx: i, rank: r})
		}
	}

	sortRanked(m.users, inCh)
	sortRanked(m.users, notInCh)

	results := make([]User, 0, min(MaxVisible, len(specials)+len(inCh)+len(notInCh)))
	results = append(results, specials...)
	for _, tier := range [][]rankedUser{inCh, notInCh} {
		for _, r := range tier {
			if len(results) == MaxVisible {
				break
			}
			results = append(results, m.users[r.idx])
		}
	}

	if len(results) > MaxVisible {
		results = results[:MaxVisible]
	}
	m.filtered = results
}

// View renders the popup at the width set by SetWidth/WithWidth, or ""
// when the picker is hidden or has no matches.
func (m *Model) View() string {
	if !m.visible || len(m.filtered) == 0 {
		return ""
	}
	s := m.styles.Chrome

	var rows []string
	for i, u := range m.filtered {
		selected := i == m.selected

		// Name color: muted for not-in-channel rows at rest;
		// upgraded to TextPrimary when selected (per spec).
		nameStyle := m.styles.Name
		if !u.InChannel && !selected {
			nameStyle = m.styles.Muted
		}
		if selected {
			nameStyle = nameStyle.Bold(true)
		}

		// Special mentions keep "(Username)" in TextPrimary (the
		// historical, intentional behavior). Regular users get no
		// parenthetical here (the dead empty-parens fix).
		label := u.DisplayName
		if u.Username != "" {
			label = fmt.Sprintf("%s (%s)", u.DisplayName, u.Username)
		}

		row := chrome.PopupMarker(s, selected) + nameStyle.Render(label)

		// Suffix in TextMuted, always (even when row is selected).
		var suffix string
		switch {
		case !u.InChannel:
			suffix = " (not in channel)"
		case u.IsExternal:
			suffix = " (ext)"
		}
		if suffix != "" {
			row += m.styles.Muted.Render(suffix)
		}

		rows = append(rows, row)
	}

	return chrome.Popup(s, rows, m.width)
}
