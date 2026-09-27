package server

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/batch"
)

// runServer is a placeholder-template server that renders small, so a batch
// test finishes quickly with no network
func runServer(t *testing.T) *Server {
	t.Helper()
	s := resolutionServer(t)
	s.jobs = make(map[string]*job)
	s.prefs.SetResolution(defaultPreviewDPI, 30)
	return s
}

func postRun(t *testing.T, s *Server, body runBody) (*httptest.ResponseRecorder, batch.View) {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/run", bytes.NewReader(raw)))
	var v batch.View
	if rec.Code == http.StatusOK {
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
	}
	return rec, v
}

// waitRun blocks until the latest run finishes
func waitRun(t *testing.T, s *Server) batch.View {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for s.runActive() {
		if time.Now().After(deadline) {
			t.Fatal("run did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The report is written just after finished is set
	run, _ := s.currentRun()
	for range 100 {
		if v := run.View(); v.Report != "" {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	return run.View()
}

func customRunRow(name, set, cn string) batch.Row {
	return batch.Row{Qty: 1, Base: card.Data{Name: name, TypeLine: "Creature", SetCode: set, CollectorNumber: cn}}
}

func TestRunWritesFilesAndReport(t *testing.T) {
	s := runServer(t)
	dir := filepath.Join(t.TempDir(), "out")
	rows := []batch.Row{
		customRunRow("Sol Ring", "c21", "263"),
		customRunRow("Sol Ring", "c21", "263"),
		{Qty: 4, Base: card.Data{Name: "Fire // Ice", TypeLine: "Instant"}, Fields: map[string]string{"power": "1"}},
	}
	rec, v := postRun(t, s, runBody{Rows: rows, OutDir: dir, Label: "test deck"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if v.ID == "" || len(v.Cards) != 3 {
		t.Fatalf("view = %+v", v)
	}
	v = waitRun(t, s)

	want := []string{"Sol Ring [C21-263].png", "Sol Ring [C21-263] (2).png", "Fire - Ice.png"}
	for i, c := range v.Cards {
		if c.Status != batch.StatusDone || c.File != want[i] {
			t.Errorf("card %d = %+v, want done as %q", i, c, want[i])
			continue
		}
		f, err := os.Open(filepath.Join(dir, c.File))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := png.DecodeConfig(f); err != nil {
			t.Errorf("%s is not a PNG: %v", c.File, err)
		}
		f.Close()
	}

	raw, err := os.ReadFile(filepath.Join(dir, v.Report))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	var rep batch.Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Counts[batch.StatusDone] != 3 || rep.Cards[2].Qty != 4 || rep.Cards[2].Fields["power"] != "1" || rep.Label != "test deck" {
		t.Errorf("report = %+v", rep)
	}

	// The image endpoint serves a finished card from disk
	img := httptest.NewRecorder()
	s.mux.ServeHTTP(img, httptest.NewRequest(http.MethodGet, "/api/run/"+v.ID+"/image/0", nil))
	if img.Code != http.StatusOK || img.Header().Get("Content-Type") != "image/png" {
		t.Errorf("image: %d %s", img.Code, img.Header().Get("Content-Type"))
	}

	// A rerun into the same folder overwrites rather than adding copies
	postRun(t, s, runBody{Rows: rows[:1], OutDir: dir})
	waitRun(t, s)
	matches, _ := filepath.Glob(filepath.Join(dir, "Sol Ring*.png"))
	if len(matches) != 2 {
		t.Errorf("after a rerun found %v", matches)
	}
}

func TestRunRefusesSecondRunAndTemplateSwitch(t *testing.T) {
	s := runServer(t)
	s.run = batch.New(nil, batch.Options{ID: "run-9"})
	rec, _ := postRun(t, s, runBody{Rows: []batch.Row{customRunRow("A", "", "")}, OutDir: t.TempDir()})
	if rec.Code != http.StatusConflict {
		t.Errorf("second run: %d, want 409", rec.Code)
	}
	sel := httptest.NewRecorder()
	s.mux.ServeHTTP(sel, httptest.NewRequest(http.MethodPost, "/api/template/select", strings.NewReader(`{"name":"normal","version":"1.0.0"}`)))
	if sel.Code != http.StatusConflict {
		t.Errorf("template switch mid-run: %d, want 409", sel.Code)
	}
}

func TestRunStop(t *testing.T) {
	s := runServer(t)
	var rows []batch.Row
	for range 40 {
		rows = append(rows, customRunRow("Card", "", ""))
	}
	_, v := postRun(t, s, runBody{Rows: rows, OutDir: t.TempDir()})
	stop := httptest.NewRecorder()
	s.mux.ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/run/"+v.ID+"/stop", nil))
	if stop.Code != http.StatusNoContent {
		t.Fatalf("stop: %d", stop.Code)
	}
	v = waitRun(t, s)
	counts := batch.Counts(v.Cards)
	if !v.Stopped || counts[batch.StatusSkipped] == 0 || counts[batch.StatusQueued] != 0 {
		t.Errorf("after stop: stopped=%v counts=%v", v.Stopped, counts)
	}
}

func TestRunRejectsBadOutput(t *testing.T) {
	s := runServer(t)
	for _, dir := range []string{"", "relative/path"} {
		if rec, _ := postRun(t, s, runBody{Rows: []batch.Row{customRunRow("A", "", "")}, OutDir: dir}); rec.Code != http.StatusBadRequest {
			t.Errorf("outDir %q: %d, want 400", dir, rec.Code)
		}
	}
}

func TestFSList(t *testing.T) {
	s := runServer(t)
	dir := t.TempDir()
	for _, d := range []string{"b", "A", ".hidden"} {
		os.Mkdir(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "file.txt"), nil, 0o644)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fs/list?path="+dir, nil))
	var got fsListing
	json.Unmarshal(rec.Body.Bytes(), &got)
	if strings.Join(got.Dirs, ",") != "A,b" || got.Parent != filepath.Dir(dir) {
		t.Errorf("listing = %+v", got)
	}
}

func TestRetryWithNoFailuresIsRefused(t *testing.T) {
	s := runServer(t)
	_, v := postRun(t, s, runBody{Rows: []batch.Row{customRunRow("Good", "", "")}, OutDir: t.TempDir()})
	waitRun(t, s)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/run/"+v.ID+"/retry", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("retry with nothing failed: %d, want 400", rec.Code)
	}
}
