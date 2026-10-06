package server

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

// runBody is the POST /api/run payload. MPC makes it an MPC Autofill project
type runBody struct {
	Rows   []batch.Row `json:"rows"`
	OutDir string      `json:"outDir"`
	Label  string      `json:"label"`
	MPC    *mpcOptions `json:"mpc,omitempty"`
}

// currentRun returns the latest run, which may have finished, and the job its
// events stream through
func (s *Server) currentRun() (*batch.Run, *job) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.run, s.runJob
}

// runActive reports whether a batch is rendering now
func (s *Server) runActive() bool {
	r, _ := s.currentRun()
	return r != nil && r.Active()
}

// handleRun starts a batch. It is refused while another is running, since two
// runs writing into overlapping folders is a class of bug better designed out
// than handled
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var body runBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad run request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.Rows) > cardlist.MaxRows {
		http.Error(w, fmt.Sprintf("a run takes at most %d cards", cardlist.MaxRows), http.StatusBadRequest)
		return
	}
	rows := batch.ExpandFaces(body.Rows)
	var project *batch.Project
	if body.MPC != nil && len(rows) > 0 {
		var err error
		if project, err = s.planProject(rows, *body.MPC); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	s.startRun(w, rows, body.OutDir, body.Label, project)
}

// handleRetryRun starts a fresh run from the cards that failed in the latest
// one, into the same folder, which is the usual fix after a network blip. A
// retried project keeps its layout, so it can finish the order file
func (s *Server) handleRetryRun(w http.ResponseWriter, r *http.Request) {
	prev, _, ok := s.runByID(w, r)
	if !ok {
		return
	}
	label := prev.Label()
	if label != "" {
		label += ", retried"
	}
	rows, project := prev.Retry()
	s.startRun(w, rows, prev.OutDir(), label, project)
}

// startRun validates a run and starts it, answering with its first snapshot
func (s *Server) startRun(w http.ResponseWriter, rows []batch.Row, outDir, label string, project *batch.Project) {
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
	// Only once no other run is writing, since it clears the old order file
	if project != nil {
		if err := project.Prepare(dir); err != nil {
			s.runMu.Unlock()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.runSeq++
	run := batch.New(rows, batch.Options{
		ID:            "run-" + strconv.FormatUint(s.runSeq, 10),
		OutDir:        dir,
		Label:         strings.TrimSpace(label),
		Template:      name,
		Version:       version,
		DPI:           dpi,
		Concurrency:   batch.Concurrency(s.prefs.Settings().Concurrency, s.autoConcurrency()),
		Compression:   batch.Compression(s.prefs.Settings().PNGCompression),
		ArtTimeout:    netTimeout,
		RenderTimeout: renderTimeout,
		Project:       project,
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
func (s *Server) handleLatestRun(w http.ResponseWriter, r *http.Request) {
	run, _ := s.currentRun()
	if run == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, run.View())
}

// runByID returns the latest run and its job when id names it. Earlier runs are
// gone
func (s *Server) runByID(w http.ResponseWriter, r *http.Request) (*batch.Run, *job, bool) {
	run, j := s.currentRun()
	if run == nil || run.ID() != r.PathValue("id") {
		http.NotFound(w, r)
		return nil, nil, false
	}
	return run, j, true
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	if _, j, ok := s.runByID(w, r); ok {
		streamJob(w, r, j)
	}
}

// handleRunImage serves one finished card from the file the run wrote, so a
// run of hundreds of cards holds none of them in memory
func (s *Server) handleRunImage(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	run, _, ok := s.runByID(w, r)
	if !ok {
		return
	}
	run.Stop()
	w.WriteHeader(http.StatusNoContent)
}

// handleOpenRunFolder opens the run's output folder in the system file browser.
// It opens only a folder a run wrote to, never a path the request names
func (s *Server) handleOpenRunFolder(w http.ResponseWriter, r *http.Request) {
	run, _, ok := s.runByID(w, r)
	if !ok {
		return
	}
	if err := s.open(run.OutDir()); err != nil {
		http.Error(w, "opening the folder: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
