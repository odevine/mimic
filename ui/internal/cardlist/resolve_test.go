package cardlist

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/carddata"
	"github.com/odevine/mimic/ui/internal/carddata/carddatatest"
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
		row        Row
		status     string
		card       string
		set        string
		candidates int
		cards      int
	}{
		{"exact name", Row{Name: "Lightning Bolt"}, StatusMatched, "Lightning Bolt", "clu", 0, 0},
		{"exact name over a shared face", Row{Name: "Swords to Plowshares"}, StatusMatched, "Swords to Plowshares", "sta", 0, 0},
		{"arena printing", Row{Name: "Lightning Bolt", Set: "2x2", Number: "117"}, StatusMatched, "Lightning Bolt", "2x2", 0, 0},
		{"missing printing falls back", Row{Name: "Lightning Bolt", Set: "zzz", Number: "1"}, StatusMatched, "Lightning Bolt", "clu", 0, 0},
		{"typo is ambiguous with the fuzzy hit first", Row{Name: "Lighning Bolt"}, StatusAmbiguous, "Lightning Bolt", "clu", 1, 0},
		{"short name offers choices", Row{Name: "Bolt"}, StatusAmbiguous, "Lightning Bolt", "clu", 3, 0},
		{"no match", Row{Name: "Nothing Here"}, StatusNotFound, "", "", 0, 0},
		{"query expands", Row{Query: "t:goblin c:r"}, StatusMatched, "", "", 0, 2},
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
	res := lookup(context.Background(), apiBackend{f}, Row{Name: "Lightning Bolt"})
	if res.Status != StatusError {
		t.Errorf("status = %q, want error", res.Status)
	}
}

func TestResolveRowCachesAndCustom(t *testing.T) {
	f := newFake()
	var cache Cache
	row := Row{Name: "Lightning Bolt"}
	resolveRow(context.Background(), APIResolver(f), &cache, row)
	before := f.calls
	if res := resolveRow(context.Background(), APIResolver(f), &cache, row); res.Status != StatusMatched || f.calls != before {
		t.Errorf("second lookup: status %q, %d new calls", res.Status, f.calls-before)
	}

	custom := Row{Name: "Big Dragon", Custom: true, Fields: map[string]string{"name": "Big Dragon", "typeLine": "Creature", "power": "9"}}
	res := resolveRow(context.Background(), APIResolver(f), &cache, custom)
	if res.Status != StatusCustom || res.Card.Name != "Big Dragon" || res.Card.Power != "9" {
		t.Errorf("custom row = %+v", res)
	}

	// A CSV row naming no real card becomes custom once it has a type line
	fallback := Row{Name: "Nothing Here", Fields: map[string]string{"name": "Nothing Here", "typeLine": "Artifact"}}
	if res := resolveRow(context.Background(), APIResolver(f), &cache, fallback); res.Status != StatusCustom || res.Card.TypeLine != "Artifact" {
		t.Errorf("fallback row = %+v", res)
	}
}

func TestResolveAllReportsEveryRow(t *testing.T) {
	rows, _, _ := Parse("Lightning Bolt\nBolt\nNothing Here\n?t:goblin c:r", "")
	var cache Cache
	var mu sync.Mutex
	seen := make(map[int]string)
	most := 0
	err := ResolveAll(context.Background(), APIResolver(newFake()), &cache, rows, func(n int, res Resolved) {
		mu.Lock()
		defer mu.Unlock()
		seen[res.Index] = res.Status
		most = max(most, n)
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{0: StatusMatched, 1: StatusAmbiguous, 2: StatusNotFound, 3: StatusMatched}
	for i, s := range want {
		if seen[i] != s {
			t.Errorf("row %d = %q, want %q", i, seen[i], s)
		}
	}
	if most != 4 {
		t.Errorf("reported %d rows done, want 4", most)
	}
}

func TestResolveAllStopsWhenCancelled(t *testing.T) {
	rows, _, _ := Parse("Lightning Bolt\nBolt", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var cache Cache
	err := ResolveAll(ctx, APIResolver(newFake()), &cache, rows, func(int, Resolved) {
		t.Error("a cancelled resolve reported a row")
	})
	if err == nil {
		t.Error("a cancelled resolve returned no error")
	}
}

func TestOverlayFields(t *testing.T) {
	base := &card.Data{Name: "Grizzly Bears", Power: "2", Toughness: "2", Colors: []card.Color{card.Green}, ArtworkURL: "u"}
	d := Overlay(base, map[string]string{"power": "3", "colors": "WG", "bogus": "x"})
	if d.Power != "3" || d.Toughness != "2" || len(d.Colors) != 2 || d.ArtworkURL != "u" {
		t.Errorf("overlay = %+v", d)
	}
	if base.Power != "2" {
		t.Error("overlay changed the base")
	}
}

// localCards builds and loads a card store from the carddatatest fixtures

func localCards(t *testing.T) *carddata.Store {

	t.Helper()

	dir := t.TempDir()

	_, err := carddata.Build(context.Background(), filepath.Join(dir, "current"), strings.NewReader(carddatatest.Oracle), strings.NewReader(carddatatest.Cards), time.Now(), nil)

	if err != nil {

		t.Fatal(err)

	}

	s := carddata.New(dir)

	s.Load()

	if !s.Ready() {

		t.Fatalf("store not ready: %+v", s.Status())

	}

	return s

}

func TestLocalResolveFallsBackToAPI(t *testing.T) {
	s := localCards(t)
	f := newFake()
	rv := LocalResolver(s, f)
	var cache Cache

	// Known locally, so the fake API is never asked
	res := resolveRow(context.Background(), rv, &cache, Row{Name: "Lightning Bolt"})
	if res.Status != StatusMatched || res.Card.SetCode != "clu" || f.calls != 0 {
		t.Errorf("local row = %+v after %d API calls", res, f.calls)
	}
	// A typo resolves locally too
	res = resolveRow(context.Background(), rv, &cache, Row{Name: "Lighning Bolt"})
	if res.Status != StatusAmbiguous || f.calls != 0 {
		t.Errorf("typo row = %+v after %d API calls", res, f.calls)
	}
	// Unknown locally but known to Scryfall, as a card newer than the copy is
	f.searches[`!"Brand New Card"`] = nil
	f.fuzzy["Brand New Card"] = bolt("new", "1")
	res = resolveRow(context.Background(), rv, &cache, Row{Name: "Brand New Card"})
	if res.Status == StatusNotFound || !strings.Contains(res.Note, "Not in the local card data") {
		t.Errorf("fallback row = %+v", res)
	}
}
