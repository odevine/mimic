package main

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
)

// runServer is a placeholder-template server that renders small, so a batch
// test finishes quickly with no network
func runServer(t *testing.T) *server {
	t.Helper()
	s := resolutionServer(t)
	s.jobs = make(map[string]*job)
	s.prefs.setResolution(defaultPreviewDPI, 30)
	return s
}

func postRun(t *testing.T, s *server, body runBody) (*httptest.ResponseRecorder, runView) {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/run", bytes.NewReader(raw)))
	var v runView
	if rec.Code == http.StatusOK {
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
	}
	return rec, v
}

// waitRun blocks until the latest run finishes
func waitRun(t *testing.T, s *server) runView {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for s.runActive() {
		if time.Now().After(deadline) {
			t.Fatal("run did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The report is written just after finished is set
	for range 100 {
		if v := s.currentRun().view(); v.Report != "" {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	return s.currentRun().view()
}

func customRunRow(name, set, cn string) runRow {
	return runRow{Qty: 1, Base: card.Data{Name: name, TypeLine: "Creature", SetCode: set, CollectorNumber: cn}}
}

func TestRunWritesFilesAndReport(t *testing.T) {
	s := runServer(t)
	dir := filepath.Join(t.TempDir(), "out")
	rows := []runRow{
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
		if c.Status != cardDone || c.File != want[i] {
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
	var rep runReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Counts[cardDone] != 3 || rep.Cards[2].Qty != 4 || rep.Cards[2].Fields["power"] != "1" || rep.Label != "test deck" {
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
	s.run = &batchRun{id: "run-9", job: &job{}}
	rec, _ := postRun(t, s, runBody{Rows: []runRow{customRunRow("A", "", "")}, OutDir: t.TempDir()})
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
	var rows []runRow
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
	counts := countCards(v.Cards)
	if !v.Stopped || counts[cardSkipped] == 0 || counts[cardQueued] != 0 {
		t.Errorf("after stop: stopped=%v counts=%v", v.Stopped, counts)
	}
}

func TestRunRejectsBadOutput(t *testing.T) {
	s := runServer(t)
	for _, dir := range []string{"", "relative/path"} {
		if rec, _ := postRun(t, s, runBody{Rows: []runRow{customRunRow("A", "", "")}, OutDir: dir}); rec.Code != http.StatusBadRequest {
			t.Errorf("outDir %q: %d, want 400", dir, rec.Code)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"Lightning Bolt":         "Lightning Bolt",
		"Fire // Ice":            "Fire - Ice",
		`What?: "A/B" <C>`:       "What-- -A-B- -C-",
		"  ..dots..  ":           "dots",
		"CON":                    "_CON",
		"tab\there":              "tabhere",
		strings.Repeat("a", 300): strings.Repeat("a", 150),
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got, _ := expandPath("~/proxies"); got != filepath.Join(home, "proxies") {
		t.Errorf("~ expanded to %q", got)
	}
	if _, err := expandPath("proxies"); err == nil {
		t.Error("a relative path should be refused")
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

func TestRetryFailedRunsOnlyFailures(t *testing.T) {
	s := runServer(t)
	dir := t.TempDir()
	rows := []runRow{customRunRow("Good", "", ""), customRunRow("Bad", "", ""), customRunRow("Also Bad", "", "")}
	_, v := postRun(t, s, runBody{Rows: rows, OutDir: dir, Label: "deck"})
	waitRun(t, s)

	// Mark two cards failed, as an art download blip would
	run := s.currentRun()
	run.mu.Lock()
	run.cards[1].Status, run.cards[2].Status = cardFailed, cardFailed
	run.mu.Unlock()

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/run/"+v.ID+"/retry", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body)
	}
	var retried runView
	json.Unmarshal(rec.Body.Bytes(), &retried)
	if len(retried.Cards) != 2 || retried.Cards[0].Name != "Bad" || retried.Label != "deck, retried" || retried.OutDir != dir {
		t.Errorf("retry run = %+v", retried)
	}
	waitRun(t, s)
}
