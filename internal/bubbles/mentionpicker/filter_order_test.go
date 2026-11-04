package mentionpicker

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/gammons/slk/internal/text"
)

// referenceFilter is the straightforward form of filter: rank every
// candidate, sort each membership tier with a total order, concatenate and
// cap. filter must return exactly this, in this order.
func referenceFilter(users []User, query string) []User {
	q := text.Fold(query)
	sq := squash(q)
	var out []User
	for _, u := range specialMentions {
		if rankUser(u, q, sq) != rankNone {
			out = append(out, u)
		}
	}
	type ranked struct {
		u User
		r matchRank
	}
	var in, notIn []ranked
	for _, u := range users {
		r := rankUser(u, q, sq)
		if r == rankNone {
			continue
		}
		if u.InChannel {
			in = append(in, ranked{u, r})
		} else {
			notIn = append(notIn, ranked{u, r})
		}
	}
	less := func(s []ranked) func(i, j int) bool {
		return func(i, j int) bool {
			if s[i].r != s[j].r {
				return s[i].r < s[j].r
			}
			if s[i].u.DisplayName != s[j].u.DisplayName {
				return s[i].u.DisplayName < s[j].u.DisplayName
			}
			return s[i].u.ID < s[j].u.ID
		}
	}
	sort.Slice(in, less(in))
	sort.Slice(notIn, less(notIn))
	for _, r := range in {
		out = append(out, r.u)
	}
	for _, r := range notIn {
		out = append(out, r.u)
	}
	if len(out) > MaxVisible {
		out = out[:MaxVisible]
	}
	return out
}

// TestFilterMatchesReference pins the picker's result list, including the
// tier order, rank order, and the display-name and ID tie-breaks, for lists
// below, at and far above MaxVisible.
func TestFilterMatchesReference(t *testing.T) {
	for _, n := range []int{0, 3, MaxVisible, MaxVisible + 1, 60} {
		users := make([]User, n)
		for i := range users {
			users[i] = User{
				ID: fmt.Sprintf("U%03d", (i*7)%n),
				// Repeated display names force the ID tie-break.
				DisplayName: fmt.Sprintf("Team-%d Member", i%4),
				Username:    fmt.Sprintf("member_%d", i),
				InChannel:   i%3 != 0,
			}
		}
		for _, q := range []string{"", "team", "mem", "member", "team1", "team-2", "zzz", "HERE", "e"} {
			m := New()
			m.SetUsers(users)
			m.Open()
			m.SetQuery(q)
			want := referenceFilter(users, q)
			if got := m.Filtered(); !reflect.DeepEqual(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Errorf("n=%d q=%q:\n got %v\nwant %v", n, q, got, want)
			}
		}
	}
}
