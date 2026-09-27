package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/cardlist"
)

// runBody is the POST /api/run payload
type runBody struct {
	Rows   []batch.Row `json:"rows"`
	OutDir string      `json:"outDir"`
	Label  string      `json:"label"`
}

// currentRun returns the latest run, which may have finished, and the job its
// events stream through
func (s *server) currentRun() (*batch.Run, *job) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.run, s.runJob
}

// runActive reports whether a batch is rendering now
func (s *server) runActive() bool {
	r, _ := s.currentRun()
	return r != nil && r.Active()
}

// handleRun starts a batch. It is refused while another is running, since two
// runs writing into overlapping folders is a class of bug better designed out
// than handled
func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	var body runBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad run request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.Rows) > cardlist.MaxRows {
		http.Error(w, fmt.Sprintf("a run takes at most %d cards", cardlist.MaxRows), http.StatusBadRequest)
		return
	}
	s.startRun(w, batch.ExpandFaces(body.Rows), body.OutDir, body.Label)
}

// handleRetryRun starts a fresh run from the cards that failed in the latest
// one, into the same folder, which is the usual fix after a network blip
func (s *server) handleRetryRun(w http.ResponseWriter, r *http.Request) {
	prev, _, ok := s.runByID(w, r)
	if !ok {
		return
	}
	label := prev.Label()
	if label != "" {
		label += ", retried"
	}
	s.startRun(w, prev.Failed(), prev.OutDir(), label)
}

// startRun validates a run and starts it, answering with its first snapshot
func (s *server) startRun(w http.ResponseWriter, rows []batch.Row, outDir, label string) {
	if len(rows) == 0 {
		http.Error(w, "the run has no cards", http.StatusBadRequest)
		return
	}
	dir, err := batch.PrepareOutDir(outDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dpi := s.renderDPI(targetOutput)
	if m, err := s.pipe.Manifest(); err == nil {
		dpi = m.ClampDPI(dpi)
	}
	name, version := s.active()
	j := &job{created: time.Now()}

	s.runMu.Lock()
	if s.run != nil && s.run.Active() {
		s.runMu.Unlock()
		http.Error(w, "a run is already in progress", http.StatusConflict)
		return
	}
	s.runSeq++
	run := batch.New(rows, batch.Options{
		ID:            "run-" + strconv.FormatUint(s.runSeq, 10),
		OutDir:        dir,
		Label:         strings.TrimSpace(label),
		Template:      name,
		Version:       version,
		DPI:           dpi,
		Concurrency:   batch.Concurrency(s.prefs.Settings().Concurrency),
		ArtTimeout:    netTimeout,
		RenderTimeout: renderTimeout,
		Emit: func(e batch.Event) {
			j.emit(jobEvent{Step: e.Step, Frac: e.Frac, Card: e.Card, Log: e.Log, Done: e.Done})
		},
	})
	s.run, s.runJob = run, j
	s.runMu.Unlock()

	go run.Execute(s.pipe)
	writeJSON(w, run.View())
}

// handleLatestRun returns the latest run, or 204 when there has been none
func (s *server) handleLatestRun(w http.ResponseWriter, r *http.Request) {
	run, _ := s.currentRun()
	if run == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, run.View())
}

// runByID returns the latest run and its job when id names it. Earlier runs are
// gone
func (s *server) runByID(w http.ResponseWriter, r *http.Request) (*batch.Run, *job, bool) {
	run, j := s.currentRun()
	if run == nil || run.ID() != r.PathValue("id") {
		http.NotFound(w, r)
		return nil, nil, false
	}
	return run, j, true
}

func (s *server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	if _, j, ok := s.runByID(w, r); ok {
		streamJob(w, r, j)
	}
}

// handleRunImage serves one finished card from the file the run wrote, so a
// run of hundreds of cards holds none of them in memory
func (s *server) handleRunImage(w http.ResponseWriter, r *http.Request) {
	run, _, ok := s.runByID(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path, ok := run.File(n)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

func (s *server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	run, _, ok := s.runByID(w, r)
	if !ok {
		return
	}
	run.Stop()
	w.WriteHeader(http.StatusNoContent)
}

// handleOpenRunFolder opens the run's output folder in the system file browser.
// It opens only a folder a run wrote to, never a path the request names
func (s *server) handleOpenRunFolder(w http.ResponseWriter, r *http.Request) {
	run, _, ok := s.runByID(w, r)
	if !ok {
		return
	}
	if err := openBrowser(run.OutDir()); err != nil {
		http.Error(w, "opening the folder: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
