package card

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card/svgpath"
)

// iconSVG is a square icon whose viewBox width says which file it came from, so a
// test can tell the rarities apart by the box that is read back
func iconSVG(width int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d 100"><path d="M0 0h%dv100H0z"/></svg>`, width, width)
}

// letterWidth is the viewBox width test icons give each rarity letter
var letterWidth = map[string]int{"C": 10, "U": 20, "R": 30, "M": 40, "T": 50, "S": 60, "B": 70}

// catalogSpec is one symbol folder in a test catalog, with the rarity letters it
// has files for
type catalogSpec map[string]string

// buildCatalog makes a catalog zip of the given folders, routes and aliases
func buildCatalog(t *testing.T, folders catalogSpec, routes, aliases map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	symbols := map[string][]string{}
	for code, letters := range folders {
		for _, l := range strings.Split(letters, "") {
			add("set/"+code+"/"+l+".svg", iconSVG(letterWidth[l]))
			symbols[code] = append(symbols[code], l)
		}
	}
	manifest := map[string]any{"set": map[string]any{"symbols": symbols, "routes": routes, "aliases": aliases}}
	raw, _ := json.Marshal(manifest)
	add("manifest.json", string(raw))
	add("set/.alt/M21-1/C.svg", iconSVG(99))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// symbolServer stands in for Scryfall's /sets list and the catalog download. sets
// maps a set code to the icon code Scryfall gives it, and the counters say how many
// requests each endpoint saw
type symbolServer struct {
	srv           *httptest.Server
	setReqs       atomic.Int32
	catalogReqs   atomic.Int32
	sets          map[string]string
	catalog       []byte
	setsStatus    int
	catalogStatus int
}

func newSymbolServer(t *testing.T, sets map[string]string, catalog []byte) *symbolServer {
	t.Helper()
	s := &symbolServer{sets: sets, catalog: catalog, setsStatus: http.StatusOK, catalogStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/sets", func(w http.ResponseWriter, r *http.Request) {
		s.setReqs.Add(1)
		if s.setsStatus != http.StatusOK {
			http.Error(w, "no", s.setsStatus)
			return
		}
		var rows []string
		for code, icon := range s.sets {
			rows = append(rows, fmt.Sprintf(`{"code":%q,"icon_svg_uri":"https://svgs.example/sets/%s.svg?1791172800"}`, code, strings.ToLower(icon)))
		}
		fmt.Fprintf(w, `{"object":"list","has_more":false,"data":[%s]}`, strings.Join(rows, ","))
	})
	mux.HandleFunc("/catalog.zip", func(w http.ResponseWriter, r *http.Request) {
		s.catalogReqs.Add(1)
		if s.catalogStatus != http.StatusOK {
			http.Error(w, "no", s.catalogStatus)
			return
		}
		w.Write(s.catalog)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *symbolServer) client(opts ...Option) *Client {
	return NewClient(append([]Option{WithBaseURL(s.srv.URL), WithSymbolCatalogURL(s.srv.URL + "/catalog.zip")}, opts...)...)
}

// defaultServer lists a few sets and a catalog that covers them
func defaultServer(t *testing.T) *symbolServer {
	cat := buildCatalog(t, catalogSpec{
		"M21": "CURM", "MH3": "CURMT", "DEFAULT": "CURM", "OLD": "C", "ODD": "CRS", "NEW": "CURMB",
	}, map[string]string{"FNM": "MH3"}, map[string]string{"ARENA": "MH3"})
	return newSymbolServer(t, map[string]string{
		"m21": "m21", "mh3": "mh3", "amh3": "mh3", // a set that takes its parent's icon
		"fnm": "dci", "pmtg2": "arena", // routed, then aliased
		"tnau": "default", "old": "old", "odd": "odd", "new": "new",
		"gone": "gone", // an icon the catalog does not have
	}, cat)
}

func fetch(t *testing.T, c *Client, set, rarity string) *SetSymbol {
	t.Helper()
	sym, err := c.FetchSetSymbol(context.Background(), &Data{SetCode: set, Rarity: rarity})
	if err != nil {
		t.Fatalf("FetchSetSymbol(%q, %q): %v", set, rarity, err)
	}
	return sym
}

func TestFetchSetSymbol(t *testing.T) {
	s := defaultServer(t)
	c := s.client()

	sym := fetch(t, c, "M21", "rare")
	if sym == nil || sym.Code != "M21" || len(sym.Icon.Shapes) != 1 {
		t.Fatalf("symbol = %+v, want M21 with one shape", sym)
	}
	if sym.Icon.ViewBox.W != float32(letterWidth["R"]) {
		t.Errorf("read the file with box width %v, want the rare one (%d)", sym.Icon.ViewBox.W, letterWidth["R"])
	}

	// Another set and another rarity reuse the index and the catalog, so one
	// request each is all it takes
	fetch(t, c, "mh3", "common")
	if s.setReqs.Load() != 1 || s.catalogReqs.Load() != 1 {
		t.Errorf("requests = %d set lists and %d catalogs, want 1 and 1", s.setReqs.Load(), s.catalogReqs.Load())
	}
	// The same set and rarity again returns the parsed symbol it already has
	if again := fetch(t, c, "M21", "rare"); again != sym {
		t.Error("second fetch parsed the icon again")
	}
}

func TestFetchSetSymbolRarity(t *testing.T) {
	s := defaultServer(t)
	c := s.client()
	tests := []struct {
		set, rarity string
		want        string
	}{
		{"m21", "common", "C"},
		{"m21", "uncommon", "U"},
		{"m21", "rare", "R"},
		{"m21", "mythic", "M"},
		{"m21", "Mythic", "M"},
		{"m21", "", "C"},
		{"m21", "unheard-of", "C"},
		{"mh3", "special", "T"}, // no special file, so the timeshifted one
		{"odd", "special", "S"},
		{"odd", "uncommon", "C"}, // no uncommon file
		{"odd", "mythic", "R"},   // no mythic file, so the next best
		{"m21", "special", "M"},
		{"new", "bonus", "B"},
		{"m21", "bonus", "M"},
		{"old", "mythic", "C"},
	}
	for _, tc := range tests {
		t.Run(tc.set+" "+tc.rarity, func(t *testing.T) {
			sym := fetch(t, c, tc.set, tc.rarity)
			if sym == nil {
				t.Fatal("no symbol")
			}
			if got := sym.Icon.ViewBox.W; got != float32(letterWidth[tc.want]) {
				t.Errorf("read the file with box width %v, want %s (%d)", got, tc.want, letterWidth[tc.want])
			}
		})
	}
}

func TestFetchSetSymbolMapping(t *testing.T) {
	s := defaultServer(t)
	c := s.client()
	tests := []struct {
		set, want string
	}{
		{"m21", "M21"},
		{"amh3", "MH3"},     // takes its parent's icon
		{"fnm", "MH3"},      // a route on the set code beats Scryfall's icon
		{"pmtg2", "MH3"},    // an alias renames the icon
		{"tnau", "DEFAULT"}, // Scryfall's placeholder icon is the catalog's default
		{"MH3", "MH3"},      // set codes are not case sensitive
		{" m21 ", "M21"},
	}
	for _, tc := range tests {
		t.Run(tc.set, func(t *testing.T) {
			sym := fetch(t, c, tc.set, "common")
			if sym == nil || sym.Code != tc.want {
				t.Errorf("symbol = %+v, want folder %s", sym, tc.want)
			}
		})
	}
}

func TestFetchSetSymbolNothingToDraw(t *testing.T) {
	s := defaultServer(t)
	c := s.client()
	tests := map[string]string{
		"no set code":            "",
		"blank set code":         "  ",
		"set Scryfall omits":     "zzz",
		"icon the catalog lacks": "gone",
	}
	for name, code := range tests {
		t.Run(name, func(t *testing.T) {
			if sym := fetch(t, c, code, "rare"); sym != nil {
				t.Errorf("symbol = %+v, want none", sym)
			}
		})
	}

	// A card with no set, or a set Scryfall does not list, needs no catalog
	fresh := s.client()
	s.catalogReqs.Store(0)
	fetch(t, fresh, "", "rare")
	fetch(t, fresh, "zzz", "rare")
	if s.catalogReqs.Load() != 0 {
		t.Errorf("downloaded the catalog %d times for sets with no symbol to look up", s.catalogReqs.Load())
	}
}

func TestFetchSetSymbolErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("set list fails", func(t *testing.T) {
		s := defaultServer(t)
		s.setsStatus = http.StatusInternalServerError
		if _, err := s.client().FetchSetSymbol(ctx, &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error when the set list fails")
		}
	})
	t.Run("a failed list is asked for again", func(t *testing.T) {
		s := defaultServer(t)
		c := s.client()
		s.setsStatus = http.StatusInternalServerError
		c.FetchSetSymbol(ctx, &Data{SetCode: "m21"})
		s.setsStatus = http.StatusOK
		if sym := fetch(t, c, "m21", "rare"); sym == nil {
			t.Error("no symbol after the list recovered")
		}
	})
	t.Run("catalog download fails", func(t *testing.T) {
		s := defaultServer(t)
		s.catalogStatus = http.StatusNotFound
		if _, err := s.client().FetchSetSymbol(ctx, &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error when the catalog cannot be downloaded")
		}
	})
	t.Run("catalog is not a zip", func(t *testing.T) {
		s := defaultServer(t)
		s.catalog = []byte("<html>not found</html>")
		if _, err := s.client().FetchSetSymbol(ctx, &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error for a catalog that is not a zip")
		}
	})
	t.Run("catalog has no manifest", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("set/M21/C.svg")
		w.Write([]byte(iconSVG(10)))
		zw.Close()
		s := defaultServer(t)
		s.catalog = buf.Bytes()
		if _, err := s.client().FetchSetSymbol(ctx, &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error for a catalog with no manifest")
		}
	})
	t.Run("icon uses unsupported features", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("set/M21/C.svg")
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path filter="url(#f)" d="M0 0h5v5z"/></svg>`))
		w, _ = zw.Create("manifest.json")
		w.Write([]byte(`{"set":{"symbols":{"M21":["C"]}}}`))
		zw.Close()
		s := defaultServer(t)
		s.catalog = buf.Bytes()
		_, err := s.client().FetchSetSymbol(ctx, &Data{SetCode: "m21"})
		if !errors.Is(err, svgpath.ErrUnsupported) {
			t.Errorf("error = %v, want one wrapping ErrUnsupported", err)
		}
	})
	t.Run("icon over the size limit", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("set/M21/C.svg")
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><!--` + strings.Repeat("x", svgpath.MaxBytes) + `--></svg>`))
		w, _ = zw.Create("manifest.json")
		w.Write([]byte(`{"set":{"symbols":{"M21":["C"]}}}`))
		zw.Close()
		s := defaultServer(t)
		s.catalog = buf.Bytes()
		if _, err := s.client().FetchSetSymbol(ctx, &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error for an oversize icon")
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		s := defaultServer(t)
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := s.client().FetchSetSymbol(cctx, &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error from a cancelled context")
		}
	})
}

func TestFetchSetSymbolCache(t *testing.T) {
	s := defaultServer(t)
	dir := t.TempDir()

	first := s.client(WithSymbolCache(NewDirCache(dir)))
	fetch(t, first, "m21", "rare")

	// A new client, as after a restart, finds both downloads on disk
	second := s.client(WithSymbolCache(NewDirCache(dir)))
	if sym := fetch(t, second, "m21", "rare"); sym == nil {
		t.Fatal("no symbol from the cache")
	}
	if s.setReqs.Load() != 1 || s.catalogReqs.Load() != 1 {
		t.Errorf("requests = %d set lists and %d catalogs, want the cache to answer the second client", s.setReqs.Load(), s.catalogReqs.Load())
	}

	// An index past its age is fetched again, and the catalog is left alone
	old := time.Now().Add(-2 * indexMaxAge)
	if err := os.Chtimes(filepath.Join(dir, setsIndexName), old, old); err != nil {
		t.Fatal(err)
	}
	fetch(t, s.client(WithSymbolCache(NewDirCache(dir))), "m21", "rare")
	if s.setReqs.Load() != 2 || s.catalogReqs.Load() != 1 {
		t.Errorf("requests = %d set lists and %d catalogs, want a fresh list and the same catalog", s.setReqs.Load(), s.catalogReqs.Load())
	}

	// A catalog past its age is downloaded again
	if err := os.Chtimes(filepath.Join(dir, catalogName), old, old); err != nil {
		t.Fatal(err)
	}
	fetch(t, s.client(WithSymbolCache(NewDirCache(dir))), "m21", "rare")
	if s.catalogReqs.Load() != 2 {
		t.Errorf("catalog requests = %d, want the old copy replaced", s.catalogReqs.Load())
	}
}

func TestFetchSetSymbolStaleCatalogWhenDownloadFails(t *testing.T) {
	s := defaultServer(t)
	dir := t.TempDir()
	fetch(t, s.client(WithSymbolCache(NewDirCache(dir))), "m21", "rare")

	old := time.Now().Add(-30 * 24 * time.Hour)
	for _, name := range []string{catalogName, setsIndexName} {
		if err := os.Chtimes(filepath.Join(dir, name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	s.catalogStatus = http.StatusServiceUnavailable
	if sym := fetch(t, s.client(WithSymbolCache(NewDirCache(dir))), "m21", "rare"); sym == nil {
		t.Error("an old catalog should still draw while a new one cannot be had")
	}
}

func TestFetchSetSymbolReplacesAnOldCatalog(t *testing.T) {
	s := defaultServer(t)
	c := s.client()
	first := fetch(t, c, "m21", "rare")
	c.symbols.loadedAt = time.Now().Add(-2 * catalogMaxAge)
	s.catalog = buildCatalog(t, catalogSpec{"M21": "CURM"}, nil, nil)
	again := fetch(t, c, "m21", "rare")
	if s.catalogReqs.Load() != 2 {
		t.Errorf("catalog requests = %d, want the old one replaced", s.catalogReqs.Load())
	}
	if again == first {
		t.Error("a replaced catalog should not keep serving icons parsed from the old one")
	}
}

func TestCatalogFolder(t *testing.T) {
	cat, err := openCatalog(buildCatalog(t, catalogSpec{"M21": "C", "MH3": "C", "DEFAULT": "C"},
		map[string]string{"FNM": "mh3"}, map[string]string{"ARENA": "MH3"}))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		set, icon, want string
		ok              bool
	}{
		{"m21", "m21", "M21", true},
		{"fnm", "dci", "MH3", true},
		{"pmtg2", "arena", "MH3", true},
		{"x", "mh3", "MH3", true},
		{"x", "nope", "NOPE", false},
		// A route to a folder the catalog does not have is no symbol
		{"x", "m21", "M21", true},
	}
	for _, tc := range tests {
		got, ok := cat.folder(tc.set, tc.icon)
		if got != tc.want || ok != tc.ok {
			t.Errorf("folder(%q, %q) = %q, %v, want %q, %v", tc.set, tc.icon, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCatalogIgnoresHiddenFolders(t *testing.T) {
	cat, err := openCatalog(buildCatalog(t, catalogSpec{"M21": "C"}, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.files["set/.alt/M21-1/C.svg"]; ok {
		t.Error("an alternate-art folder was indexed")
	}
	if len(cat.files) != 1 {
		t.Errorf("indexed %d files, want 1", len(cat.files))
	}
}

func TestDirCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	c := NewDirCache(dir)

	if _, ok := c.Get("a.svg", 0); ok {
		t.Error("Get hit before any Put")
	}
	c.Put("a.svg", []byte("one"))
	if got, ok := c.Get("a.svg", 0); !ok || string(got) != "one" {
		t.Errorf("Get = %q, %v, want one", got, ok)
	}
	c.Put("a.svg", []byte("two"))
	if got, _ := c.Get("a.svg", time.Hour); string(got) != "two" {
		t.Errorf("Get after overwrite = %q, want two", got)
	}

	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "a.svg"), old, old); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("a.svg", time.Minute); ok {
		t.Error("Get accepted an entry older than maxAge")
	}
	if _, ok := c.Get("a.svg", 0); !ok {
		t.Error("Get with no maxAge should accept any age")
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("left a temporary file behind: %s", e.Name())
		}
	}
}

func TestDirCache_RejectsOddNames(t *testing.T) {
	dir := t.TempDir()
	c := NewDirCache(dir)
	for _, name := range []string{"", "../x", "a/b", ".hidden"} {
		c.Put(name, []byte("x"))
		if _, ok := c.Get(name, 0); ok {
			t.Errorf("Get(%q) hit, want names that are not plain files refused", name)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) > 0 {
		t.Errorf("cache directory holds %d entries after odd names", len(entries))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "x")); err == nil {
		t.Error("a name escaped the cache directory")
	}
}

func TestDirCache_NoDirCachesNothing(t *testing.T) {
	for _, c := range []*DirCache{nil, NewDirCache(""), {}} {
		c.Put("a.svg", []byte("x"))
		if _, ok := c.Get("a.svg", 0); ok {
			t.Error("a cache with no directory returned an entry")
		}
	}
}
