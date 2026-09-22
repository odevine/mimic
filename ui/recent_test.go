package main

import (
	"strings"
	"testing"
)

func TestRecentsAddNewestFirst(t *testing.T) {
	r := &recents{}
	r.add("bolt")
	r.add("delver")
	got := strings.Join(r.list(), ",")
	if got != "delver,bolt" {
		t.Errorf("list = %q, want delver,bolt", got)
	}
}

func TestRecentsDeduplicatesCaseInsensitive(t *testing.T) {
	r := &recents{}
	r.add("Lightning Bolt")
	r.add("delver")
	r.add("lightning bolt")
	got := r.list()
	if len(got) != 2 {
		t.Fatalf("list = %v, want 2 entries", got)
	}
	// The re-used query moves to the front, keeping the newest casing
	if got[0] != "lightning bolt" || got[1] != "delver" {
		t.Errorf("list = %v, want [lightning bolt delver]", got)
	}
}

func TestRecentsCapsAtMax(t *testing.T) {
	r := &recents{}
	for i := 0; i < maxRecent+5; i++ {
		r.add(string(rune('a' + i)))
	}
	if len(r.list()) != maxRecent {
		t.Errorf("len = %d, want %d", len(r.list()), maxRecent)
	}
}

func TestRecentsIgnoresBlank(t *testing.T) {
	r := &recents{}
	r.add("   ")
	r.add("")
	if len(r.list()) != 0 {
		t.Errorf("list = %v, want empty", r.list())
	}
}

func TestNewRecentsPreservesOrderAndTrims(t *testing.T) {
	// Stored newest-first should round-trip to the same order
	r := newRecents([]string{"delver", "bolt", "delver"})
	got := r.list()
	if len(got) != 2 || got[0] != "delver" || got[1] != "bolt" {
		t.Errorf("list = %v, want [delver bolt]", got)
	}
}
