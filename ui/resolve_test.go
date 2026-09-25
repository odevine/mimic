package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/odevine/mimic/engine/card"
)

// fakeSource answers lookups from fixed tables and counts calls
type fakeSource struct {
	mu       sync.Mutex
	searches map[string][]*card.Data
	fuzzy    map[string]*card.Data
	fail     bool
	calls    int
}

func (f *fakeSource) Search(ctx context.Context, q string) ([]*card.Data, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail {
		return nil, errors.New("scryfall: searching: connection refused")
	}
	return f.searches[q], nil
}

func (f *fakeSource) FetchByName(ctx context.Context, name string) (*card.Data, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if d, ok := f.fuzzy[name]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("scryfall: fetching %q: unexpected status 404 Not Found", name)
}

func bolt(set, cn string) *card.Data {
	return &card.Data{Name: "Lightning Bolt", SetCode: set, CollectorNumber: cn}
}

func newFake() *fakeSource {
	return &fakeSource{
		searches: map[string][]*card.Data{
			`!"Lightning Bolt"`:                              {bolt("clu", "141")},
			`!"Swords to Plowshares"`:                        {{Name: "Emeritus of Truce"}, {Name: "Swords to Plowshares", SetCode: "sta"}},
			`!"Lightning Bolt" set:2x2 cn:117 unique:prints`: {bolt("2x2", "117")},
			`Bolt order:edhrec`:                              {bolt("clu", "141"), {Name: "Boltwing Marauder"}, {Name: "Chain Lightning"}},
			`t:goblin c:r`:                                   {{Name: "Goblin Guide"}, {Name: "Goblin Bushwhacker"}},
			`Lighning Bolt order:edhrec`:                     nil,
			`!"Nothing Here"`:                                nil,
		},
		fuzzy: map[string]*card.Data{"Lighning Bolt": bolt("clu", "141")},
	}
}

func TestLookup(t *testing.T) {
	cases := []struct {
		name       string
		row        listRow
		status     string
		card       string
		set        string
		candidates int
		cards      int
	}{
		{"exact name", listRow{Name: "Lightning Bolt"}, rowMatched, "Lightning Bolt", "clu", 0, 0},
		{"exact name over a shared face", listRow{Name: "Swords to Plowshares"}, rowMatched, "Swords to Plowshares", "sta", 0, 0},
		{"arena printing", listRow{Name: "Lightning Bolt", Set: "2x2", Number: "117"}, rowMatched, "Lightning Bolt", "2x2", 0, 0},
		{"missing printing falls back", listRow{Name: "Lightning Bolt", Set: "zzz", Number: "1"}, rowMatched, "Lightning Bolt", "clu", 0, 0},
		{"typo is ambiguous with the fuzzy hit first", listRow{Name: "Lighning Bolt"}, rowAmbiguous, "Lightning Bolt", "clu", 1, 0},
		{"short name offers choices", listRow{Name: "Bolt"}, rowAmbiguous, "Lightning Bolt", "clu", 3, 0},
		{"no match", listRow{Name: "Nothing Here"}, rowNotFound, "", "", 0, 0},
		{"query expands", listRow{Query: "t:goblin c:r"}, rowMatched, "", "", 0, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := lookup(context.Background(), apiBackend{newFake()}, c.row)
			if res.Status != c.status {
				t.Fatalf("status = %q, want %q (%s)", res.Status, c.status, res.Note)
			}
			if c.card != "" && (res.Card == nil || res.Card.Name != c.card || res.Card.SetCode != c.set) {
				t.Errorf("card = %+v, want %s from %s", res.Card, c.card, c.set)
			}
			if len(res.Candidates) != c.candidates || len(res.Cards) != c.cards {
				t.Errorf("candidates %d cards %d, want %d and %d", len(res.Candidates), len(res.Cards), c.candidates, c.cards)
			}
		})
	}
}

func TestLookupNetworkErrorIsNotNotFound(t *testing.T) {
	f := newFake()
	f.fail = true
	res := lookup(context.Background(), apiBackend{f}, listRow{Name: "Lightning Bolt"})
	if res.Status != rowError {
		t.Errorf("status = %q, want error", res.Status)
	}
}

func TestResolveRowCachesAndCustom(t *testing.T) {
	f := newFake()
	var cache resolveCache
	row := listRow{Name: "Lightning Bolt"}
	resolveRow(context.Background(), apiResolver(f), &cache, row)
	before := f.calls
	if res := resolveRow(context.Background(), apiResolver(f), &cache, row); res.Status != rowMatched || f.calls != before {
		t.Errorf("second lookup: status %q, %d new calls", res.Status, f.calls-before)
	}

	custom := listRow{Name: "Big Dragon", Custom: true, Fields: map[string]string{"name": "Big Dragon", "typeLine": "Creature", "power": "9"}}
	res := resolveRow(context.Background(), apiResolver(f), &cache, custom)
	if res.Status != rowCustom || res.Card.Name != "Big Dragon" || res.Card.Power != "9" {
		t.Errorf("custom row = %+v", res)
	}

	// A CSV row naming no real card becomes custom once it has a type line
	fallback := listRow{Name: "Nothing Here", Fields: map[string]string{"name": "Nothing Here", "typeLine": "Artifact"}}
	if res := resolveRow(context.Background(), apiResolver(f), &cache, fallback); res.Status != rowCustom || res.Card.TypeLine != "Artifact" {
		t.Errorf("fallback row = %+v", res)
	}
}

func TestResolveRowsStreamsEveryRow(t *testing.T) {
	rows, _, _ := parseList("Lightning Bolt\nBolt\nNothing Here\n?t:goblin c:r", "")
	j := &job{}
	var cache resolveCache
	resolveRows(context.Background(), j, apiResolver(newFake()), &cache, rows)

	events, finished, _ := j.stream(0)
	if !finished {
		t.Fatal("job did not finish")
	}
	seen := make(map[int]string)
	for _, e := range events {
		if e.Row != nil {
			seen[e.Row.Index] = e.Row.Status
		}
	}
	want := map[int]string{0: rowMatched, 1: rowAmbiguous, 2: rowNotFound, 3: rowMatched}
	for i, s := range want {
		if seen[i] != s {
			t.Errorf("row %d = %q, want %q", i, seen[i], s)
		}
	}
	last := events[len(events)-1]
	if !last.Done || last.Err != "" || !strings.Contains(last.Step, "4 of 4") {
		t.Errorf("last event = %+v", last)
	}
}

func TestOverlayFields(t *testing.T) {
	base := &card.Data{Name: "Grizzly Bears", Power: "2", Toughness: "2", Colors: []card.Color{card.Green}, ArtworkURL: "u"}
	d := overlayFields(base, map[string]string{"power": "3", "colors": "WG", "bogus": "x"})
	if d.Power != "3" || d.Toughness != "2" || len(d.Colors) != 2 || d.ArtworkURL != "u" {
		t.Errorf("overlay = %+v", d)
	}
	if base.Power != "2" {
		t.Error("overlay changed the base")
	}
}

func apiResolver(src cardSource) resolver {
	return resolver{mode: cardDataAPI, primary: apiBackend{src}}
}
