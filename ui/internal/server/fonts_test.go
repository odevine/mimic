package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/ui/internal/fontdir"
)

// fontUpload builds a PUT /api/fonts/{folder} request carrying raw as the file
func fontUpload(t *testing.T, role, name string, raw []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(raw)
	mw.Close()
	req := httptest.NewRequest(http.MethodPut, "/api/fonts/"+role, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("folder", role)
	return req
}

func decodeFonts(t *testing.T, rec *httptest.ResponseRecorder) fontsStatus {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var st fontsStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func roleLine(st fontsStatus, role string) fontRole {
	for _, r := range st.Roles {
		if r.Role == role {
			return r
		}
	}
	return fontRole{}
}

func TestFontsAddAndRemove(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "engine", "fonts", "embedded", "body", "Merriweather-Italic.ttf"))
	if err != nil {
		t.Skipf("engine fonts not in this checkout: %v", err)
	}
	s := &Server{fonts: fontdir.Dir{Path: t.TempDir(), Kind: fontdir.KindManaged}}

	rec := httptest.NewRecorder()
	s.handleFonts(rec, httptest.NewRequest(http.MethodGet, "/api/fonts", nil))
	st := decodeFonts(t, rec)
	if !st.Writable || len(st.Roles) != 6 {
		t.Fatalf("status = %+v, want a writable folder with six roles", st)
	}
	if got := roleLine(st, "info"); got.Source != "borrowed" || got.Borrows != "body" {
		t.Errorf("info = %+v, want borrowed from body", got)
	}

	rec = httptest.NewRecorder()
	s.handlePutFont(rec, fontUpload(t, "title", "Mine.ttf", raw))
	if got := roleLine(decodeFonts(t, rec), "title"); got.Source != "user" || got.File != "Mine.ttf" || got.Style != "Italic" {
		t.Errorf("title after upload = %+v, want Mine.ttf from the user", got)
	}

	rec = httptest.NewRecorder()
	s.handlePutFont(rec, fontUpload(t, "title", "Bad.ttf", []byte("nope")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad upload status %d, want 400", rec.Code)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/fonts/title", nil)
	req.SetPathValue("folder", "title")
	rec = httptest.NewRecorder()
	s.handleDeleteFont(rec, req)
	if got := roleLine(decodeFonts(t, rec), "title"); got.Source != "default" {
		t.Errorf("title after remove = %+v, want the default", got)
	}
}

func TestFontsReadOnlyRefusesWrites(t *testing.T) {
	s := &Server{fonts: fontdir.Dir{Path: t.TempDir(), Kind: fontdir.KindCheckout}}
	rec := httptest.NewRecorder()
	s.handlePutFont(rec, fontUpload(t, "title", "Mine.ttf", []byte("x")))
	if rec.Code != http.StatusConflict {
		t.Errorf("upload to a checkout: status %d, want 409", rec.Code)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/fonts/title", nil)
	req.SetPathValue("folder", "title")
	rec = httptest.NewRecorder()
	s.handleDeleteFont(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("remove from a checkout: status %d, want 409", rec.Code)
	}
}

func TestSymbolRevalidatesByETag(t *testing.T) {
	s := &Server{fonts: fontdir.Dir{Path: t.TempDir(), Kind: fontdir.KindManaged}}
	get := func(etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/symbol?code=W&px=32", nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		s.handleSymbol(rec, req)
		return rec
	}
	first := get("")
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" {
		t.Fatalf("status %d etag %q, want a pip with an ETag: %s", first.Code, etag, first.Body)
	}
	if again := get(etag); again.Code != http.StatusNotModified {
		t.Errorf("revalidation status %d, want 304", again.Code)
	}
}
