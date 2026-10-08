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
	"time"

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

func control(s *Server, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1:1"+path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestAReleaseAppearsAndClears(t *testing.T) {
	s := newServer(t)
	if rec := control(s, "PUT", "/__fake/release", `{"version":"1.1.0","notes":"Fixes"}`); rec.Code != 204 {
		t.Fatalf("PUT release = %d %s", rec.Code, rec.Body)
	}
	rec := get(s, GitHubAPI, "/repos/odevine/mimic/releases")
	for _, want := range []string{`"ui/v1.1.0"`, `"Fixes"`, "SHA256SUMS", installer("1.1.0")} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("releases lack %q: %s", want, rec.Body)
		}
	}
	if sums := get(s, GitHubFiles, "/odevine/mimic/releases/download/ui/v1.1.0/SHA256SUMS"); !strings.Contains(sums.Body.String(), installer("1.1.0")) {
		t.Errorf("checksums = %s", sums.Body)
	}
	control(s, "DELETE", "/__fake/release", "")
	if rec := get(s, GitHubAPI, "/repos/odevine/mimic/releases"); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("a cleared release still answers: %s", rec.Body)
	}
}

func TestAReleaseNeedsAVersion(t *testing.T) {
	s := newServer(t)
	if rec := control(s, "PUT", "/__fake/release", `{"notes":"x"}`); rec.Code != 400 {
		t.Errorf("a release with no version = %d", rec.Code)
	}
}

func TestAHostCanBeToldToFail(t *testing.T) {
	s := newServer(t)
	control(s, "PUT", "/__fake/fail", `{"host":"api.scryfall.com","status":503}`)
	if rec := get(s, ScryfallAPI, "/cards/search?q=bolt"); rec.Code != 503 {
		t.Errorf("a failing host = %d, want 503", rec.Code)
	}
	if rec := get(s, ScryfallImages, "/art_crop/x.jpg"); rec.Code != 200 {
		t.Errorf("another host = %d, want it unaffected", rec.Code)
	}
	control(s, "DELETE", "/__fake/fail", "")
	if rec := get(s, ScryfallAPI, "/cards/search?q=bolt"); rec.Code != 200 {
		t.Errorf("after clearing = %d", rec.Code)
	}
	if rec := control(s, "PUT", "/__fake/fail", `{"host":"x","status":200}`); rec.Code != 400 {
		t.Errorf("a failure with a success status = %d, want 400", rec.Code)
	}
}

func TestAHostCanBeToldToWait(t *testing.T) {
	s := newServer(t)
	control(s, "PUT", "/__fake/delay", `{"host":"cards.scryfall.io","ms":150}`)
	start := time.Now()
	get(s, ScryfallImages, "/art_crop/x.jpg")
	if took := time.Since(start); took < 150*time.Millisecond {
		t.Errorf("a slow host answered in %v", took)
	}
	start = time.Now()
	get(s, ScryfallAPI, "/bulk-data")
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Errorf("another host waited %v", took)
	}
	control(s, "DELETE", "/__fake/delay", "")
	start = time.Now()
	get(s, ScryfallImages, "/art_crop/x.jpg")
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Errorf("after clearing it still waited %v", took)
	}
	if rec := control(s, "PUT", "/__fake/delay", `{"host":"x","ms":0}`); rec.Code != 400 {
		t.Errorf("a zero delay = %d, want 400", rec.Code)
	}
}

func TestControlCallsAreNotRecordedAsTheAppsRequests(t *testing.T) {
	s := newServer(t)
	control(s, "PUT", "/__fake/release", `{"version":"2.0.0"}`)
	get(s, ScryfallAPI, "/cards/search?q=bolt")
	if reqs := s.Requests(); len(reqs) != 1 {
		t.Errorf("requests = %v, want only the app's", reqs)
	}
	var listed []string
	rec := control(s, "GET", "/__fake/requests", "")
	json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed) != 1 {
		t.Errorf("GET requests = %v", listed)
	}
	control(s, "DELETE", "/__fake/requests", "")
	if len(s.Requests()) != 0 {
		t.Error("requests were not forgotten")
	}
}

func TestTheCatalogListsTheBundlesItIsToldOf(t *testing.T) {
	s := newServer(t)
	rec := get(s, GitHubRaw, "/odevine/mimic-templates/main/index.json")
	if strings.Contains(rec.Body.String(), `"normal"`) {
		t.Error("the catalog starts with a template")
	}
	if rec := control(s, "PUT", "/__fake/catalog", `{"templates":[{"name":"normal","version":"1.0.0"},{"name":"normal","version":"1.1.0"}]}`); rec.Code != 204 {
		t.Fatalf("PUT catalog = %d %s", rec.Code, rec.Body)
	}
	var idx struct {
		Templates []struct {
			Name, Latest string
			Versions     []struct{ Version, URL string }
		}
	}
	rec = get(s, GitHubRaw, "/odevine/mimic-templates/main/index.json")
	if err := json.Unmarshal(rec.Body.Bytes(), &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Templates) != 1 || idx.Templates[0].Latest != "1.1.0" || idx.Templates[0].Versions[0].Version != "1.1.0" {
		t.Errorf("index = %+v, want one template whose newest version is first", idx.Templates)
	}
	if rec := control(s, "PUT", "/__fake/catalog", `{"templates":[{"name":"nope","version":"1.0.0"}]}`); rec.Code != 400 {
		t.Errorf("an unknown template = %d, want 400", rec.Code)
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
