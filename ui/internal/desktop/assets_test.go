package desktop

import (
	"errors"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/odevine/mimic/ui/internal/apierr"
)

// fakeImages is an Images with a render, a run file, a cardback and one symbol
type fakeImages struct{ dir string }

func (f fakeImages) RenderImage(id string) (image.Image, string, error) {
	if id != "7" {
		return nil, "", apierr.New(apierr.NotFound, "no such render")
	}
	return image.NewRGBA(image.Rect(0, 0, 4, 4)), "Sol Ring", nil
}

func (f fakeImages) RunFile(id string, n int) (string, error) {
	if id != "run-1" || n != 0 {
		return "", apierr.New(apierr.NotFound, "no such card")
	}
	return filepath.Join(f.dir, "card.jpg"), nil
}

func (f fakeImages) CardbackFile() (string, error) {
	return "", apierr.New(apierr.NotFound, "no cardback")
}

func (f fakeImages) Symbol(code string, px int, known string) (image.Image, string, error) {
	if code != "W" {
		return nil, "", errors.New("unknown symbol")
	}
	if known == `"tag"` {
		return nil, `"tag"`, nil
	}
	return image.NewRGBA(image.Rect(0, 0, px, px)), `"tag"`, nil
}

func serve(h http.Handler, method, url string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAssetHandlerServesImagesAndFrontend(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "card.jpg"), []byte("jpeg bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := assetHandler(fakeImages{dir}, fstest.MapFS{"index.html": {Data: []byte("<title>Mimic</title>")}}, false)

	if rec := serve(h, "GET", "/img/render/7"); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("render: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"))
	}
	if rec := serve(h, "GET", "/img/render/8"); rec.Code != 404 {
		t.Errorf("unknown render: %d, want 404", rec.Code)
	}
	if rec := serve(h, "GET", "/img/run/run-1/0"); rec.Code != 200 || rec.Body.String() != "jpeg bytes" || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("run file: %d %q", rec.Code, rec.Body)
	}
	if rec := serve(h, "GET", "/img/run/run-1/x"); rec.Code != 404 {
		t.Errorf("a card number that is not a number: %d, want 404", rec.Code)
	}
	if rec := serve(h, "GET", "/img/run/run-2/0"); rec.Code != 404 {
		t.Errorf("another run: %d, want 404", rec.Code)
	}
	if rec := serve(h, "GET", "/img/cardback"); rec.Code != 404 {
		t.Errorf("no cardback: %d, want 404", rec.Code)
	}
	if rec := serve(h, "GET", "/"); rec.Code != 200 || rec.Body.String() != "<title>Mimic</title>" {
		t.Errorf("frontend: %d %q", rec.Code, rec.Body)
	}
	if rec := serve(h, "GET", "/smoke.html"); rec.Code != 404 {
		t.Errorf("the launch check page is served outside a launch check: %d", rec.Code)
	}
}

func TestAssetHandlerSymbolRevalidates(t *testing.T) {
	h := assetHandler(fakeImages{}, fstest.MapFS{}, false)
	first := serve(h, "GET", "/img/symbol?code=W&px=24")
	if first.Code != 200 || first.Header().Get("ETag") != `"tag"` || first.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("first: %d %v", first.Code, first.Header())
	}
	if again := serve(h, "GET", "/img/symbol?code=W&px=24", "If-None-Match", `"tag"`); again.Code != 304 {
		t.Errorf("revalidation: %d, want 304", again.Code)
	}
	if rec := serve(h, "GET", "/img/symbol?code=NOPE"); rec.Code != 500 {
		t.Errorf("an error with no kind: %d, want 500", rec.Code)
	}
}

func TestAssetHandlerServesTheLaunchCheckOnlyWhenAsked(t *testing.T) {
	h := assetHandler(fakeImages{}, fstest.MapFS{}, true)
	if rec := serve(h, "GET", "/smoke.html"); rec.Code != 200 || rec.Body.Len() == 0 {
		t.Errorf("launch check page: %d", rec.Code)
	}
}
