package carddata

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

	"github.com/odevine/mimic/ui/internal/carddata/carddatatest"
)

// fixtureStore builds and loads a store from the fixtures
func fixtureStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	meta, err := Build(context.Background(), filepath.Join(dir, "current"), strings.NewReader(carddatatest.Oracle), strings.NewReader(carddatatest.Cards), time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Printings != 8 {
		t.Errorf("built %d printings, want 8 with the art series card left out", meta.Printings)
	}
	s := New(dir)
	s.Load()
	if !s.Ready() {
		t.Fatalf("store not ready: %+v", s.Status())
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
		d, err := s.Exact(c.name)
		if err != nil || d == nil || !strings.HasPrefix(d.Name, c.want) || d.SetCode != c.set {
			t.Errorf("exact(%q) = %+v, %v, want %s from %s", c.name, d, err, c.want, c.set)
		}
	}
	if d, _ := s.Exact("Nothing Here"); d != nil {
		t.Errorf("exact of an unknown name = %+v", d)
	}
}

func TestLocalArtURLFromID(t *testing.T) {
	s := fixtureStore(t)
	d, _ := s.Printing("Lightning Bolt", "2x2", "117")
	if d == nil || d.ArtworkURL != "https://cards.scryfall.io/art_crop/front/b/o/bolt-2x2.jpg" {
		t.Errorf("art url = %+v", d)
	}
}

func TestLocalPrinting(t *testing.T) {
	s := fixtureStore(t)
	if d, _ := s.Printing("Lightning Bolt", "2X2", "117"); d == nil || d.CollectorNumber != "117" {
		t.Errorf("printing 2X2 117 = %+v", d)
	}
	if d, _ := s.Printing("Lightning Bolt", "clu", ""); d == nil || d.SetCode != "clu" {
		t.Errorf("printing in clu = %+v", d)
	}
	// A set and number that belong to another card do not match
	if d, _ := s.Printing("Lightning Bolt", "frc", "37"); d != nil {
		t.Errorf("mismatched printing = %+v", d)
	}
	if d, _ := s.Printing("Lightning Bolt", "zzz", "1"); d != nil {
		t.Errorf("missing printing = %+v", d)
	}
}

func TestLocalPrintingsNewestFirst(t *testing.T) {
	s := fixtureStore(t)
	ps, err := s.Printings("Lightning Bolt")
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
	sim, _ := s.Similar("bolt", 8)
	var names []string
	for _, d := range sim {
		names = append(names, d.Name)
	}
	if strings.Join(names, ",") != "Lightning Bolt,Boltwave" {
		t.Errorf("similar = %v, want most played first", names)
	}
	if d, _ := s.Fuzzy("Lighning Bolt"); d == nil || d.Name != "Lightning Bolt" {
		t.Errorf("fuzzy typo = %+v", d)
	}
	if d, _ := s.Fuzzy("Completely Different"); d != nil {
		t.Errorf("fuzzy far miss = %+v", d)
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

func TestFetchInstallsACopy(t *testing.T) {
	oracle, cards := gzipped(carddatatest.Oracle), gzipped(carddatatest.Cards)
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

	ctx := context.Background()
	info, err := FetchRemote(ctx, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(oracle)+len(cards)) || info.UpdatedAt.Day() != 23 {
		t.Errorf("remote = %+v", info)
	}

	s := New(t.TempDir())
	if !s.BeginDownload("first") {
		t.Fatal("download refused")
	}
	if s.BeginDownload("again") {
		t.Error("a second download should be refused")
	}
	if st := s.Status(); st.JobID != "first" {
		t.Errorf("status during a download = %+v", st)
	}
	var done int64
	meta, err := s.Fetch(ctx, srv.Client(), info, func(n, total int64) { done = n })
	if err != nil {
		t.Fatal(err)
	}
	if meta.Printings != 8 || done != info.Size {
		t.Errorf("fetched %d printings after %d of %d bytes", meta.Printings, done, info.Size)
	}
	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	s.EndDownload()
	st := s.Status()
	if st.State != StateReady || st.Printings != 8 || st.UpdatedAt.Day() != 23 || st.JobID != "" {
		t.Errorf("status after download = %+v", st)
	}
	if d, _ := s.Exact("Lightning Bolt"); d == nil {
		t.Error("the installed copy does not answer lookups")
	}

	// Downloading again replaces the copy in place
	if _, err := s.Fetch(ctx, srv.Client(), info, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Install(); err != nil || s.Status().State != StateReady {
		t.Errorf("second install: %v, %+v", err, s.Status())
	}

	if err := s.Remove(); err != nil || s.Status().State != StateNone {
		t.Errorf("remove: %v, %+v", err, s.Status())
	}
}
