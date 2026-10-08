package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/pipeline/pipelinetest"
	"github.com/odevine/mimic/ui/internal/services/cards"
	"github.com/odevine/mimic/ui/internal/services/data"
	"github.com/odevine/mimic/ui/internal/services/list"
	"github.com/odevine/mimic/ui/internal/services/render"
	"github.com/odevine/mimic/ui/internal/services/run"
	"github.com/odevine/mimic/ui/internal/services/settings"
	"github.com/odevine/mimic/ui/internal/services/templates"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

func TestMain(m *testing.M) { pipelinetest.Run(m, &pipeline.WritePlaceholders) }

// testServer is the HTTP face of services over placeholder assets
func testServer(t *testing.T) *Server {
	t.Helper()
	ws := workspacetest.NewSmall(t)
	open := func(string) error { return nil }
	return New(Services{
		Workspace: ws,
		Cards:     cards.New(ws),
		Render:    render.New(ws),
		List:      list.New(ws),
		Run:       run.New(ws, open),
		Templates: templates.New(ws),
		Settings:  settings.New(ws),
		Data:      data.New(ws, open),
	}, nil)
}

func get(s *Server, url string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

func TestFSList(t *testing.T) {
	s := testServer(t)
	dir := t.TempDir()
	for _, d := range []string{"b", "A", ".hidden"} {
		os.Mkdir(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "file.txt"), nil, 0o644)

	var got fsListing
	json.Unmarshal(get(s, "/api/fs/list?path="+dir).Body.Bytes(), &got)
	if strings.Join(got.Dirs, ",") != "A,b" || got.Parent != filepath.Dir(dir) {
		t.Errorf("listing = %+v", got)
	}
}

func TestRoutesAnswerWithTheStatusOfTheirErrorKind(t *testing.T) {
	s := testServer(t)
	for url, want := range map[string]int{
		"/api/capabilities":         http.StatusOK,
		"/api/settings":             http.StatusOK,
		"/api/resolution":           http.StatusOK,
		"/api/template/active":      http.StatusOK,
		"/api/run":                  http.StatusNoContent,
		"/api/render/nope/image":    http.StatusNotFound,
		"/api/render/nope/events":   http.StatusNotFound,
		"/api/run/run-1/image/0":    http.StatusNotFound,
		"/api/mpc/cardback":         http.StatusNotFound,
		"/api/symbol?code=W&px=32":  http.StatusOK,
		"/api/symbol?code=NO&px=32": http.StatusNotFound,
	} {
		if rec := get(s, url); rec.Code != want {
			t.Errorf("GET %s = %d, want %d: %s", url, rec.Code, want, rec.Body)
		}
	}
}

func TestPostRunBadBodyIsRefused(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/run", strings.NewReader("{")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestResolveStreamsOverSSE(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/resolve", strings.NewReader(`{"text":""}`)))
	var started struct{ JobID string }
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil || started.JobID == "" {
		t.Fatalf("resolve = %d %s", rec.Code, rec.Body)
	}
	events := get(s, "/api/resolve/"+started.JobID+"/events")
	if ct := events.Header().Get("Content-Type"); ct != "text/event-stream" || !strings.Contains(events.Body.String(), `"seq":1`) {
		t.Errorf("events: %s %q", ct, events.Body)
	}
}
