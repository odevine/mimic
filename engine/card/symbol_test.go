package card

import (
	"context"
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

const squareSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><path d="M10 10h80v80H10z"/></svg>`

// symbolServer stands in for Scryfall's /sets list and its icon host. icons
// maps a set code to the SVG it serves, a code mapped to "" is listed with the
// placeholder icon, and the counters say how many requests each endpoint saw
type symbolServer struct {
	srv        *httptest.Server
	sets, svgs atomic.Int32
	icons      map[string]string
	setsStatus int
	svgStatus  int
}

func newSymbolServer(t *testing.T, icons map[string]string) *symbolServer {
	t.Helper()
	s := &symbolServer{icons: icons, setsStatus: http.StatusOK, svgStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/sets", func(w http.ResponseWriter, r *http.Request) {
		s.sets.Add(1)
		if s.setsStatus != http.StatusOK {
			http.Error(w, "no", s.setsStatus)
			return
		}
		var rows []string
		for code, svg := range s.icons {
			uri := s.srv.URL + "/svgs/" + code + ".svg?1791172800"
			if svg == "" {
				uri = s.srv.URL + "/svgs/default.svg?1791172800"
			}
			rows = append(rows, fmt.Sprintf(`{"code":%q,"icon_svg_uri":%q}`, code, uri))
		}
		fmt.Fprintf(w, `{"object":"list","has_more":false,"data":[%s]}`, strings.Join(rows, ","))
	})
	mux.HandleFunc("/svgs/", func(w http.ResponseWriter, r *http.Request) {
		s.svgs.Add(1)
		if s.svgStatus != http.StatusOK {
			http.Error(w, "no", s.svgStatus)
			return
		}
		code := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/svgs/"), ".svg")
		w.Header().Set("Content-Type", "image/svg+xml")
		fmt.Fprint(w, s.icons[code])
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *symbolServer) client(opts ...Option) *Client {
	return NewClient(append([]Option{WithBaseURL(s.srv.URL)}, opts...)...)
}

func TestFetchSetSymbol(t *testing.T) {
	s := newSymbolServer(t, map[string]string{"m21": squareSVG, "lea": squareSVG})
	c := s.client()

	sym, err := c.FetchSetSymbol(context.Background(), &Data{SetCode: "M21"})
	if err != nil {
		t.Fatalf("FetchSetSymbol: %v", err)
	}
	if sym == nil || sym.Code != "m21" || len(sym.Icon.Shapes) != 1 {
		t.Fatalf("symbol = %+v, want m21 with one shape", sym)
	}
	if sym.Icon.ViewBox != (svgpath.Rect{W: 100, H: 100}) {
		t.Errorf("ViewBox = %+v, want 0 0 100 100", sym.Icon.ViewBox)
	}

	// The same set again reuses the parsed icon, and another set reuses the
	// index, so one list request and one icon request per set is all it takes
	if again, err := c.FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err != nil || again != sym {
		t.Errorf("second fetch = %v, %v, want the same symbol", again, err)
	}
	if _, err := c.FetchSetSymbol(context.Background(), &Data{SetCode: "lea"}); err != nil {
		t.Fatalf("FetchSetSymbol lea: %v", err)
	}
	if s.sets.Load() != 1 || s.svgs.Load() != 2 {
		t.Errorf("requests = %d lists and %d icons, want 1 and 2", s.sets.Load(), s.svgs.Load())
	}
}

func TestFetchSetSymbol_NothingToDraw(t *testing.T) {
	s := newSymbolServer(t, map[string]string{"m21": squareSVG, "tnau": ""})
	c := s.client()
	tests := map[string]string{
		"no set code":          "",
		"blank set code":       "  ",
		"set Scryfall omits":   "zzz",
		"placeholder icon set": "tnau",
	}
	for name, code := range tests {
		t.Run(name, func(t *testing.T) {
			sym, err := c.FetchSetSymbol(context.Background(), &Data{SetCode: code})
			if sym != nil || err != nil {
				t.Errorf("FetchSetSymbol = %v, %v, want nil and no error", sym, err)
			}
		})
	}
	if s.svgs.Load() != 0 {
		t.Errorf("downloaded %d icons, want none for a set with no symbol", s.svgs.Load())
	}
}

func TestFetchSetSymbol_NoCodeMakesNoRequest(t *testing.T) {
	s := newSymbolServer(t, map[string]string{"m21": squareSVG})
	if _, err := s.client().FetchSetSymbol(context.Background(), &Data{}); err != nil {
		t.Fatal(err)
	}
	if s.sets.Load() != 0 {
		t.Errorf("made %d list requests for a card with no set", s.sets.Load())
	}
}

func TestFetchSetSymbol_Errors(t *testing.T) {
	t.Run("set list fails", func(t *testing.T) {
		s := newSymbolServer(t, map[string]string{"m21": squareSVG})
		s.setsStatus = http.StatusInternalServerError
		if _, err := s.client().FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error when the set list fails")
		}
	})
	t.Run("a failed list is asked for again", func(t *testing.T) {
		s := newSymbolServer(t, map[string]string{"m21": squareSVG})
		c := s.client()
		s.setsStatus = http.StatusInternalServerError
		c.FetchSetSymbol(context.Background(), &Data{SetCode: "m21"})
		s.setsStatus = http.StatusOK
		if sym, err := c.FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err != nil || sym == nil {
			t.Errorf("after recovery = %v, %v, want a symbol", sym, err)
		}
	})
	t.Run("icon download fails", func(t *testing.T) {
		s := newSymbolServer(t, map[string]string{"m21": squareSVG})
		s.svgStatus = http.StatusNotFound
		if _, err := s.client().FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error when the icon download fails")
		}
	})
	t.Run("icon is not usable SVG", func(t *testing.T) {
		s := newSymbolServer(t, map[string]string{"m21": "<html>not found</html>"})
		if _, err := s.client().FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error for a file that is not SVG")
		}
	})
	t.Run("icon uses unsupported features", func(t *testing.T) {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path filter="url(#f)" d="M0 0h5v5z"/></svg>`
		s := newSymbolServer(t, map[string]string{"frc": svg})
		_, err := s.client().FetchSetSymbol(context.Background(), &Data{SetCode: "frc"})
		if !errors.Is(err, svgpath.ErrUnsupported) {
			t.Errorf("error = %v, want one wrapping ErrUnsupported", err)
		}
	})
	t.Run("icon over the size limit", func(t *testing.T) {
		big := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><!--` + strings.Repeat("x", svgpath.MaxBytes) + `--></svg>`
		s := newSymbolServer(t, map[string]string{"big": big})
		if _, err := s.client().FetchSetSymbol(context.Background(), &Data{SetCode: "big"}); err == nil {
			t.Error("want an error for an oversize icon")
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		s := newSymbolServer(t, map[string]string{"m21": squareSVG})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.client().FetchSetSymbol(ctx, &Data{SetCode: "m21"}); err == nil {
			t.Error("want an error from a cancelled context")
		}
	})
}

func TestFetchSetSymbol_Cache(t *testing.T) {
	s := newSymbolServer(t, map[string]string{"m21": squareSVG})
	dir := t.TempDir()

	first := s.client(WithSymbolCache(NewDirCache(dir)))
	if _, err := first.FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err != nil {
		t.Fatal(err)
	}

	// A new client, as after a restart, finds both downloads on disk
	second := s.client(WithSymbolCache(NewDirCache(dir)))
	sym, err := second.FetchSetSymbol(context.Background(), &Data{SetCode: "m21"})
	if err != nil || sym == nil {
		t.Fatalf("cached fetch = %v, %v, want a symbol", sym, err)
	}
	if s.sets.Load() != 1 || s.svgs.Load() != 1 {
		t.Errorf("requests = %d lists and %d icons, want the cache to answer the second client", s.sets.Load(), s.svgs.Load())
	}

	// An index past its age is fetched again, while the icon, which is named
	// for its version, is not
	old := time.Now().Add(-2 * setsIndexMaxAge)
	if err := os.Chtimes(filepath.Join(dir, setsIndexName), old, old); err != nil {
		t.Fatal(err)
	}
	third := s.client(WithSymbolCache(NewDirCache(dir)))
	if _, err := third.FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err != nil {
		t.Fatal(err)
	}
	if s.sets.Load() != 2 || s.svgs.Load() != 1 {
		t.Errorf("requests = %d lists and %d icons, want a fresh list and the same icon", s.sets.Load(), s.svgs.Load())
	}
}

func TestFetchSetSymbol_IndexRefreshesWhenOld(t *testing.T) {
	s := newSymbolServer(t, map[string]string{"m21": squareSVG})
	c := s.client()
	if _, err := c.FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err != nil {
		t.Fatal(err)
	}
	c.symbols.indexedAt = time.Now().Add(-2 * setsIndexMaxAge)
	if _, err := c.FetchSetSymbol(context.Background(), &Data{SetCode: "m21"}); err != nil {
		t.Fatal(err)
	}
	if s.sets.Load() != 2 {
		t.Errorf("list requests = %d, want the old index replaced", s.sets.Load())
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

func TestVersionToken(t *testing.T) {
	tests := map[string]string{
		"1791172800":       "1791172800",
		"":                 "0",
		"../../etc/passwd": "etcpasswd",
		"v=1&x=2":          "v1x2",
	}
	for in, want := range tests {
		if got := versionToken(in); got != want {
			t.Errorf("versionToken(%q) = %q, want %q", in, got, want)
		}
	}
}
