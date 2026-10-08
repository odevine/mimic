package fakeupstream

import (
	"encoding/json"
	"image"
	_ "image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/odevine/mimic/ui/testassets"
	"io/fs"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	dir, err := fs.Sub(testassets.Scryfall, "scryfall")
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func get(s *Server, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "http://"+host+path, nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func search(t *testing.T, s *Server, q string) (int, []string) {
	t.Helper()
	rec := get(s, ScryfallAPI, "/cards/search?q="+url.QueryEscape(q))
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	var list struct {
		Data []struct{ Name, Set string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range list.Data {
		out = append(out, c.Name+"/"+c.Set)
	}
	return rec.Code, out
}

func TestSearchMatchesNameWords(t *testing.T) {
	s := newServer(t)
	_, got := search(t, s, "lightning bolt")
	if strings.Join(got, ",") != "Lightning Bolt/2x2,Lightning Bolt/m11" {
		t.Errorf("lightning bolt = %v", got)
	}
	_, got = search(t, s, "ELVES")
	if len(got) != 1 || got[0] != "Llanowar Elves/dom" {
		t.Errorf("elves = %v", got)
	}
}

func TestSearchExactNamePrintingsAndFilters(t *testing.T) {
	s := newServer(t)
	_, got := search(t, s, `!"Lightning Bolt" unique:prints`)
	if len(got) != 2 {
		t.Errorf("printings = %v, want both", got)
	}
	_, got = search(t, s, "set:m11 bolt")
	if len(got) != 1 || got[0] != "Lightning Bolt/m11" {
		t.Errorf("set filter = %v", got)
	}
	_, got = search(t, s, "t:artifact")
	if len(got) != 1 || got[0] != "Sol Ring/c21" {
		t.Errorf("type filter = %v", got)
	}
	_, got = search(t, s, `!"Delver of Secrets"`)
	if len(got) != 1 {
		t.Errorf("a double-faced card is found by its front name: %v", got)
	}
}

func TestSearchWithNoMatchIsAScryfallNotFound(t *testing.T) {
	s := newServer(t)
	rec := get(s, ScryfallAPI, "/cards/search?q=zzzz")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Errorf("no match = %d %s", rec.Code, rec.Body)
	}
}

func TestSearchRefusesATermItDoesNotUnderstand(t *testing.T) {
	s := newServer(t)
	code, _ := search(t, s, "c:red cmc=1")
	if code != http.StatusBadRequest {
		t.Errorf("an unsupported operator answered %d, want 400", code)
	}
}

func TestNamedLookup(t *testing.T) {
	s := newServer(t)
	rec := get(s, ScryfallAPI, "/cards/named?fuzzy=sol+ring")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"Sol Ring"`) {
		t.Errorf("named = %d %s", rec.Code, rec.Body)
	}
	if rec := get(s, ScryfallAPI, "/cards/named?exact=nothing"); rec.Code != 404 {
		t.Errorf("a missing card answered %d", rec.Code)
	}
}

func TestArtIsADecodableImageThatDependsOnThePath(t *testing.T) {
	s := newServer(t)
	a := get(s, ScryfallImages, "/art_crop/front/a.jpg")
	b := get(s, ScryfallImages, "/art_crop/front/b.jpg")
	again := get(s, ScryfallImages, "/art_crop/front/a.jpg")
	if _, _, err := image.Decode(strings.NewReader(a.Body.String())); err != nil {
		t.Fatalf("art does not decode: %v", err)
	}
	if a.Body.String() != again.Body.String() {
		t.Error("the same path gave different art")
	}
	if a.Body.String() == b.Body.String() {
		t.Error("two paths gave the same art")
	}
}

func TestGitHubHostsAnswerAsAnEmptyWorld(t *testing.T) {
	s := newServer(t)
	if rec := get(s, GitHubAPI, "/repos/odevine/mimic/releases?per_page=50"); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("releases = %s", rec.Body)
	}
	rec := get(s, GitHubRaw, "/odevine/mimic-templates/main/index.json")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"templates"`) {
		t.Errorf("index = %d %s", rec.Code, rec.Body)
	}
}

func TestAnUnplannedHostIsABadGatewayAndIsRecorded(t *testing.T) {
	s := newServer(t)
	rec := get(s, "example.com:443", "/x")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "example.com") {
		t.Errorf("unplanned host = %d %s", rec.Code, rec.Body)
	}
	if reqs := s.Requests(); len(reqs) != 1 || reqs[0] != "example.com /x" {
		t.Errorf("requests = %v", reqs)
	}
}
