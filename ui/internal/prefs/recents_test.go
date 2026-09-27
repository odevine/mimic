package prefs

import (
	"strings"
	"testing"
)

func TestRecentsAddNewestFirst(t *testing.T) {
	r := &Recents{}
	r.Add("bolt")
	r.Add("delver")
	got := strings.Join(r.List(), ",")
	if got != "delver,bolt" {
		t.Errorf("list = %q, want delver,bolt", got)
	}
}

func TestRecentsDeduplicatesCaseInsensitive(t *testing.T) {
	r := &Recents{}
	r.Add("Lightning Bolt")
	r.Add("delver")
	r.Add("lightning bolt")
	got := r.List()
	if len(got) != 2 {
		t.Fatalf("list = %v, want 2 entries", got)
	}
	// The re-used query moves to the front, keeping the newest casing
	if got[0] != "lightning bolt" || got[1] != "delver" {
		t.Errorf("list = %v, want [lightning bolt delver]", got)
	}
}

func TestRecentsCapsAtMax(t *testing.T) {
	r := &Recents{}
	for i := 0; i < maxRecent+5; i++ {
		r.Add(string(rune('a' + i)))
	}
	if len(r.List()) != maxRecent {
		t.Errorf("len = %d, want %d", len(r.List()), maxRecent)
	}
}

func TestRecentsIgnoresBlank(t *testing.T) {
	r := &Recents{}
	r.Add("   ")
	r.Add("")
	if len(r.List()) != 0 {
		t.Errorf("list = %v, want empty", r.List())
	}
}

func TestNewRecentsPreservesOrderAndTrims(t *testing.T) {
	// Stored newest-first should round-trip to the same order
	r := NewRecents([]string{"delver", "bolt", "delver"})
	got := r.List()
	if len(got) != 2 || got[0] != "delver" || got[1] != "bolt" {
		t.Errorf("list = %v, want [delver bolt]", got)
	}
}
