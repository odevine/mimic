// Package run is the service behind the Run flow: batch renders of a whole list,
// optionally laid out as an MPC Autofill project
package run

import (
	"strconv"
	"strings"
	"sync"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// Service starts, stops and reports on batch runs. Only the latest run is kept
type Service struct {
	ws   *workspace.Workspace
	open func(path string) error

	// cardbackMu keeps two cardback uploads from mixing one's image with the
	// other's name
	cardbackMu sync.Mutex

	// finished is told how each run ended
	finished func(batch.View)
}

// OnFinished sets a function called, off the run's own goroutine, with the final
// state of every run when it ends. Set it before any run starts
func (s *Service) OnFinished(f func(batch.View)) { s.finished = f }

// New returns the service over a workspace. open shows a folder in the system
// file browser
func New(ws *workspace.Workspace, open func(path string) error) *Service {
	return &Service{ws: ws, open: open}
}

// Shutdown stops the run in progress, so the pipeline can close under it
func (s *Service) Shutdown() {
	if r, _ := s.ws.Run.Current(); r != nil && r.Active() {
		r.Stop()
	}
}

// Request is a run: the rows to render and where to put them. MPC makes it an
// MPC Autofill project
type Request struct {
	Rows   []batch.Row `json:"rows"`
	OutDir string      `json:"outDir"`
	Label  string      `json:"label"`
	MPC    *MPCOptions `json:"mpc,omitempty"`
}

// Start begins a batch. It is refused while another is running, since two runs
// writing into overlapping folders is a class of bug better designed out than
// handled
func (s *Service) Start(req Request) (batch.View, error) {
	if len(req.Rows) > cardlist.MaxRows {
		return batch.View{}, apierr.Newf(apierr.BadRequest, "a run takes at most %d cards", cardlist.MaxRows)
	}
	rows := batch.ExpandFaces(req.Rows)
	var project *batch.Project
	if req.MPC != nil && len(rows) > 0 {
		var err error
		if project, err = s.planProject(rows, *req.MPC); err != nil {
			return batch.View{}, apierr.Wrap(apierr.BadRequest, "", err)
		}
	}
	return s.start(rows, req.OutDir, req.Label, project)
}

// Retry starts a fresh run from the cards that failed in the latest one, into
// the same folder, which is the usual fix after a network blip. A retried
// project keeps its layout, so it can finish the order file
func (s *Service) Retry(id string) (batch.View, error) {
	prev, _, err := s.byID(id)
	if err != nil {
		return batch.View{}, err
	}
	label := prev.Label()
	if label != "" {
		label += ", retried"
	}
	rows, project := prev.Retry()
	return s.start(rows, prev.OutDir(), label, project)
}

// start validates a run and starts it, answering with its first snapshot
func (s *Service) start(rows []batch.Row, outDir, label string, project *batch.Project) (batch.View, error) {
	if len(rows) == 0 {
		return batch.View{}, apierr.New(apierr.BadRequest, "the run has no cards")
	}
	dir, err := batch.PrepareOutDir(outDir)
	if err != nil {
		return batch.View{}, apierr.Wrap(apierr.BadRequest, "", err)
	}
	dpi := s.ws.ClampedDPI(workspace.TargetOutput)
	name, version := s.ws.Active()
	settings := s.ws.Prefs.Settings()

	var run *batch.Run
	started, err := s.ws.Run.Begin(func(seq uint64, prev *jobs.Job) (*batch.Run, *jobs.Job, error) {
		// Only once no other run is writing, since it clears the old order file
		if project != nil {
			if err := project.Prepare(dir); err != nil {
				return nil, nil, apierr.Wrap(apierr.Internal, "", err)
			}
		}
		if prev != nil {
			s.ws.Jobs.Forget(prev.ID())
		}
		id := "run-" + strconv.FormatUint(seq, 10)
		j := s.ws.Jobs.NewNamed(id)
		run = batch.New(rows, batch.Options{
			ID:            id,
			OutDir:        dir,
			Label:         strings.TrimSpace(label),
			Template:      name,
			Version:       version,
			DPI:           dpi,
			Concurrency:   batch.Concurrency(settings.Concurrency, s.ws.AutoConcurrency()),
			Format:        settings.ImageFormat,
			Compression:   batch.Compression(settings.PNGCompression),
			ArtTimeout:    workspace.NetTimeout,
			RenderTimeout: workspace.RenderTimeout,
			Project:       project,
			Emit: func(e batch.Event) {
				j.Emit(jobs.Event{Step: e.Step, Frac: e.Frac, Card: e.Card, Log: e.Log, Done: e.Done, Warmup: e.Warmup, WarmupCards: e.WarmupCards})
				if e.Done && s.finished != nil {
					go s.finished(run.View())
				}
			},
		})
		return run, j, nil
	})
	if err != nil {
		return batch.View{}, err
	}
	if !started {
		return batch.View{}, apierr.New(apierr.Conflict, "a run is already in progress")
	}
	go run.Execute(s.ws.Pipe)
	return run.View(), nil
}

// Latest returns the latest run, or nil when there has been none
func (s *Service) Latest() *batch.View {
	run, _ := s.ws.Run.Current()
	if run == nil {
		return nil
	}
	v := run.View()
	return &v
}

// byID returns the latest run and its job when id names it. Earlier runs are gone
func (s *Service) byID(id string) (*batch.Run, *jobs.Job, error) {
	run, j := s.ws.Run.Current()
	if run == nil || run.ID() != id {
		return nil, nil, apierr.New(apierr.NotFound, "no such run")
	}
	return run, j, nil
}

// Job returns the job a run's events go through
func (s *Service) Job(id string) (*jobs.Job, error) {
	_, j, err := s.byID(id)
	return j, err
}

// File returns the path of one finished card, from the file the run wrote, so a
// run of hundreds of cards holds none of them in memory
func (s *Service) File(id string, n int) (string, error) {
	run, _, err := s.byID(id)
	if err != nil {
		return "", err
	}
	path, ok := run.File(n)
	if !ok {
		return "", apierr.New(apierr.NotFound, "no such card")
	}
	return path, nil
}

// Stop ends the run, keeping the cards already finished
func (s *Service) Stop(id string) error {
	run, _, err := s.byID(id)
	if err != nil {
		return err
	}
	run.Stop()
	return nil
}

// OpenFolder opens the run's output folder in the system file browser. It opens
// only a folder a run wrote to, never a path the caller names
func (s *Service) OpenFolder(id string) error {
	run, _, err := s.byID(id)
	if err != nil {
		return err
	}
	if err := s.open(run.OutDir()); err != nil {
		return apierr.Wrap(apierr.Internal, "opening the folder", err)
	}
	return nil
}
