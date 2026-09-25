package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Bulk data lines in the shape Scryfall writes them, trimmed to what matters
const oracleFixture = `{"id":"bolt-clu","oracle_id":"o-bolt","name":"Lightning Bolt"}
{"id":"stp-frc","oracle_id":"o-stp","name":"Swords to Plowshares"}
`

const cardsFixture = `{"id":"bolt-2x2","oracle_id":"o-bolt","name":"Lightning Bolt","layout":"normal","set":"2x2","collector_number":"117","released_at":"2022-07-08","games":["paper"],"edhrec_rank":5,"mana_cost":"{R}","type_line":"Instant","colors":["R"],"image_uris":{"art_crop":"x"}}
{"id":"bolt-clu","oracle_id":"o-bolt","name":"Lightning Bolt","layout":"normal","set":"clu","collector_number":"141","released_at":"2024-02-23","games":["paper"],"edhrec_rank":5,"mana_cost":"{R}","type_line":"Instant","colors":["R"]}
{"id":"bolt-promo","oracle_id":"o-bolt","name":"Lightning Bolt","layout":"normal","set":"plst","collector_number":"1","released_at":"2025-01-01","promo":true,"games":["paper"],"edhrec_rank":5,"type_line":"Instant"}
{"id":"bolt-art","name":"Lightning Bolt // Lightning Bolt","layout":"art_series","set":"a2x2","collector_number":"1","released_at":"2022-07-08","games":["paper"]}
{"id":"stp-frc","oracle_id":"o-stp","name":"Swords to Plowshares","layout":"normal","set":"frc","collector_number":"37","released_at":"2025-01-01","games":["paper"],"edhrec_rank":20,"type_line":"Instant"}
{"id":"emeritus","oracle_id":"o-em","name":"Emeritus of Truce // Swords to Plowshares","layout":"modal_dfc","set":"sos","collector_number":"13","released_at":"2026-04-01","games":["paper"],"edhrec_rank":900,"type_line":"Creature // Instant","card_faces":[{"name":"Emeritus of Truce"},{"name":"Swords to Plowshares"}]}
{"id":"vault","oracle_id":"o-vault","name":"Lim-Dûl's Vault","layout":"normal","set":"c13","collector_number":"197","released_at":"2013-11-01","games":["paper"],"edhrec_rank":3000,"type_line":"Instant"}
{"id":"boltwave","oracle_id":"o-wave","name":"Boltwave","layout":"normal","set":"fdn","collector_number":"79","released_at":"2024-11-15","games":["paper"],"edhrec_rank":40,"type_line":"Sorcery"}
{"id":"rev","name":"Sol Ring // Sol Ring","layout":"reversible_card","set":"sld","collector_number":"999","released_at":"2023-01-01","games":["paper"],"card_faces":[{"name":"Sol Ring","oracle_id":"o-sol"},{"name":"Sol Ring","oracle_id":"o-sol"}]}
`

// fixtureStore builds and loads a store from the fixtures
func fixtureStore(t *testing.T) *localStore {
	t.Helper()
	dir := t.TempDir()
	meta, err := buildLocalCards(context.Background(), filepath.Join(dir, "current"), strings.NewReader(oracleFixture), strings.NewReader(cardsFixture), time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Printings != 8 {
		t.Errorf("built %d printings, want 8 with the art series card left out", meta.Printings)
	}
	s := newLocalStore(dir)
	s.load()
	if !s.ready() {
		t.Fatalf("store not ready: %+v", s.status())
	}
	return s
}

func TestLocalExact(t *testing.T) {
	s := fixtureStore(t)
	cases := []struct {
		name, want, set string
	}{
		// Scryfall's own default printing wins over a newer promo or an older printing
		{"Lightning Bolt", "Lightning Bolt", "clu"},
		{"lightning  BOLT", "Lightning Bolt", "clu"},
		// A card whose own name matches beats a double-faced card with that back face
		{"Swords to Plowshares", "Swords to Plowshares", "frc"},
		{"Emeritus of Truce", "Emeritus of Truce", "sos"},
		{"Lim-Dul's Vault", "Lim-Dûl's Vault", "c13"},
		// A reversible card carries its oracle id on its faces
		{"Sol Ring", "Sol Ring", "sld"},
	}
	for _, c := range cases {
		d, err := s.exact(c.name)
		if err != nil || d == nil || !strings.HasPrefix(d.Name, c.want) || d.SetCode != c.set {
			t.Errorf("exact(%q) = %+v, %v, want %s from %s", c.name, d, err, c.want, c.set)
		}
	}
	if d, _ := s.exact("Nothing Here"); d != nil {
		t.Errorf("exact of an unknown name = %+v", d)
	}
}

func TestLocalArtURLFromID(t *testing.T) {
	s := fixtureStore(t)
	d, _ := s.printing("Lightning Bolt", "2x2", "117")
	if d == nil || d.ArtworkURL != "https://cards.scryfall.io/art_crop/front/b/o/bolt-2x2.jpg" {
		t.Errorf("art url = %+v", d)
	}
}

func TestLocalPrinting(t *testing.T) {
	s := fixtureStore(t)
	if d, _ := s.printing("Lightning Bolt", "2X2", "117"); d == nil || d.CollectorNumber != "117" {
		t.Errorf("printing 2X2 117 = %+v", d)
	}
	if d, _ := s.printing("Lightning Bolt", "clu", ""); d == nil || d.SetCode != "clu" {
		t.Errorf("printing in clu = %+v", d)
	}
	// A set and number that belong to another card do not match
	if d, _ := s.printing("Lightning Bolt", "frc", "37"); d != nil {
		t.Errorf("mismatched printing = %+v", d)
	}
	if d, _ := s.printing("Lightning Bolt", "zzz", "1"); d != nil {
		t.Errorf("missing printing = %+v", d)
	}
}

func TestLocalPrintingsNewestFirst(t *testing.T) {
	s := fixtureStore(t)
	ps, err := s.printings("Lightning Bolt")
	if err != nil {
		t.Fatal(err)
	}
	var sets []string
	for _, p := range ps {
		sets = append(sets, p.SetCode)
	}
	if strings.Join(sets, ",") != "plst,clu,2x2" {
		t.Errorf("printings = %v", sets)
	}
}

func TestLocalSimilarAndFuzzy(t *testing.T) {
	s := fixtureStore(t)
	sim, _ := s.similar("bolt", 8)
	var names []string
	for _, d := range sim {
		names = append(names, d.Name)
	}
	if strings.Join(names, ",") != "Lightning Bolt,Boltwave" {
		t.Errorf("similar = %v, want most played first", names)
	}
	if d, _ := s.fuzzy("Lighning Bolt"); d == nil || d.Name != "Lightning Bolt" {
		t.Errorf("fuzzy typo = %+v", d)
	}
	if d, _ := s.fuzzy("Completely Different"); d != nil {
		t.Errorf("fuzzy far miss = %+v", d)
	}
}

func TestLocalResolveFallsBackToAPI(t *testing.T) {
	s := fixtureStore(t)
	api := apiBackend{newFake()}
	rv := resolver{mode: cardDataLocal, primary: localBackend{store: s, api: api}, fallback: api}
	var cache resolveCache

	// Known locally, so the fake API is never asked
	f := api.src.(*fakeSource)
	res := resolveRow(context.Background(), rv, &cache, listRow{Name: "Lightning Bolt"})
	if res.Status != rowMatched || res.Card.SetCode != "clu" || f.calls != 0 {
		t.Errorf("local row = %+v after %d API calls", res, f.calls)
	}
	// A typo resolves locally too
	res = resolveRow(context.Background(), rv, &cache, listRow{Name: "Lighning Bolt"})
	if res.Status != rowAmbiguous || f.calls != 0 {
		t.Errorf("typo row = %+v after %d API calls", res, f.calls)
	}
	// Unknown locally but known to Scryfall, as a card newer than the copy is
	f.searches[`!"Brand New Card"`] = nil
	f.fuzzy["Brand New Card"] = bolt("new", "1")
	res = resolveRow(context.Background(), rv, &cache, listRow{Name: "Brand New Card"})
	if res.Status == rowNotFound || !strings.Contains(res.Note, "Not in the local card data") {
		t.Errorf("fallback row = %+v", res)
	}
}

func TestEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"bolt", "bolt", 0},
		{"lighning bolt", "lightning bolt", 1},
		{"abc", "xyz", 3},
		{"short", "a much longer name", 3},
	}
	for _, c := range cases {
		if got := editDistance(c.a, c.b, 2); min(got, 3) != min(c.want, 3) {
			t.Errorf("editDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// gzipped compresses a fixture the way Scryfall serves bulk files
func gzipped(s string) []byte {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	z.Write([]byte(s))
	z.Close()
	return b.Bytes()
}

func TestDownloadCardsInstallsACopy(t *testing.T) {
	oracle, cards := gzipped(oracleFixture), gzipped(cardsFixture)
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/bulk-data", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":[
			{"type":"oracle_cards","updated_at":"2026-09-23T21:01:58Z","jsonl_download_uri":"%[1]s/oracle.jsonl.gz","compressed_size":%[2]d},
			{"type":"default_cards","updated_at":"2026-09-23T21:05:38Z","jsonl_download_uri":"%[1]s/default.jsonl.gz","compressed_size":%[3]d}]}`,
			srv.URL, len(oracle), len(cards))
	})
	mux.HandleFunc("/oracle.jsonl.gz", func(w http.ResponseWriter, r *http.Request) { w.Write(oracle) })
	mux.HandleFunc("/default.jsonl.gz", func(w http.ResponseWriter, r *http.Request) { w.Write(cards) })
	srv = httptest.NewServer(mux)
	defer srv.Close()
	old := bulkListURL
	bulkListURL = srv.URL + "/bulk-data"
	t.Cleanup(func() { bulkListURL = old })

	s := &server{cards: newLocalStore(t.TempDir()), scryfall: srv.Client(), jobs: make(map[string]*job)}
	id, j := s.newJob()
	if !s.cards.beginDownload(id) {
		t.Fatal("download refused")
	}
	if s.cards.beginDownload("again") {
		t.Error("a second download should be refused")
	}
	s.downloadCards(j)

	events, finished, _ := j.stream(0)
	last := events[len(events)-1]
	if !finished || last.Err != "" {
		t.Fatalf("download ended with %+v", last)
	}
	st := s.cards.status()
	if st.State != cardDataReady || st.Printings != 8 || st.UpdatedAt.Day() != 23 || st.JobID != "" {
		t.Errorf("status after download = %+v", st)
	}
	if d, _ := s.cards.exact("Lightning Bolt"); d == nil {
		t.Error("the installed copy does not answer lookups")
	}

	// Downloading again replaces the copy in place
	id, j = s.newJob()
	s.cards.beginDownload(id)
	s.downloadCards(j)
	if st := s.cards.status(); st.State != cardDataReady {
		t.Errorf("status after a second download = %+v", st)
	}

	if err := s.cards.remove(); err != nil || s.cards.status().State != cardDataNone {
		t.Errorf("remove: %v, %+v", err, s.cards.status())
	}
}
