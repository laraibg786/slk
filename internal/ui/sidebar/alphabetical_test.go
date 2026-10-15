package sidebar

import (
	"reflect"
	"testing"
)

func filteredIDs(m *Model) []string {
	var ids []string
	for _, idx := range m.filtered {
		ids = append(ids, m.items[idx].ID)
	}
	return ids
}

func TestChannelsSortAlphabeticallyWithinSection(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "random", Type: "channel"},
		{ID: "C2", Name: "general", Type: "channel"},
		{ID: "C3", Name: "eng", Type: "channel"},
	})
	want := []string{"C3", "C2", "C1"}
	if got := filteredIDs(&m); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestAlphabeticalSortIgnoresCaseAndAccents(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "Zulu", Type: "channel"},
		{ID: "C2", Name: "école", Type: "channel"},
		{ID: "C3", Name: "Bravo", Type: "channel"},
		{ID: "C4", Name: "alpha", Type: "channel"},
	})
	want := []string{"C4", "C3", "C2", "C1"}
	if got := filteredIDs(&m); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestExplicitChannelOrderBeatsAlphabeticalOrder(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "alpha", Type: "channel", Section: "Eng"},
		{ID: "C2", Name: "zulu", Type: "channel", Section: "Eng", ChannelOrder: 1},
		{ID: "C3", Name: "mike", Type: "channel", Section: "Eng", ChannelOrder: 2},
		{ID: "C4", Name: "bravo", Type: "channel", Section: "Eng"},
	})
	want := []string{"C2", "C3", "C1", "C4"}
	if got := filteredIDs(&m); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestAlphabeticalSortKeepsSectionOrder(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "zulu", Type: "channel", Section: "A"},
		{ID: "C2", Name: "alpha", Type: "channel", Section: "B"},
		{ID: "C3", Name: "bravo", Type: "channel", Section: "A"},
	})
	m.SetSectionsProvider(&fakeProvider{
		ready: true,
		sections: []SectionMeta{
			{ID: "A", Name: "First", Type: "standard"},
			{ID: "B", Name: "Second", Type: "standard"},
		},
	})
	want := []string{"C3", "C1", "C2"}
	if got := filteredIDs(&m); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestAlphabeticalSortKeepsStarBlocks(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "D1", Name: "Ann", Type: "dm", Section: "ST"},
		{ID: "C1", Name: "zulu", Type: "channel", Section: "ST"},
		{ID: "C2", Name: "mike", Type: "private", Section: "ST"},
		{ID: "D2", Name: "Zed", Type: "dm", Section: "ST"},
	})
	m.SetSectionsProvider(&fakeProvider{ready: true, sections: []SectionMeta{{ID: "ST", Type: "stars"}}})
	want := []string{"C2", "C1", "D1", "D2"}
	if got := filteredIDs(&m); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestEqualNamesKeepInputOrder(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "Same", Type: "channel"},
		{ID: "C2", Name: "alpha", Type: "channel"},
		{ID: "C3", Name: "same", Type: "channel"},
		{ID: "C4", Name: "SAME", Type: "channel"},
	})
	want := []string{"C2", "C1", "C3", "C4"}
	if got := filteredIDs(&m); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestCursorStaysOnItemWhenAlphabeticalOrderChanges(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "bravo", Type: "channel"},
		{ID: "C2", Name: "delta", Type: "channel"},
		{ID: "C3", Name: "echo", Type: "channel"},
	})
	m.SelectByID("C2")
	oldCursor := m.cursor

	// A rename sorts C2 to the front of its section.
	m.UpsertItem(ChannelItem{ID: "C2", Name: "alpha", Type: "channel"})
	if got, want := filteredIDs(&m), []string{"C2", "C1", "C3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if m.SelectedID() != "C2" {
		t.Fatalf("selected %q after re-sort, want C2", m.SelectedID())
	}
	if m.cursor == oldCursor {
		t.Fatal("re-sort did not move the selected row's index")
	}
}
