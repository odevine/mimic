// Package list is the service behind the From a List flow
package list

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// maxListBytes bounds a list file read from disk. A deck list is a few
// kilobytes, so this only stops a wrong file from being read whole
const maxListBytes = 8 << 20

// Service parses a pasted or opened list and resolves its rows to cards
type Service struct {
	ws *workspace.Workspace

	// resolveCancel abandons the resolve in flight when a new one starts, and
	// resolved caches lookups for the session
	mu            sync.Mutex
	resolveCancel context.CancelFunc
	resolved      cardlist.Cache
}

// New returns the service over a workspace
func New(ws *workspace.Workspace) *Service { return &Service{ws: ws} }

// Request is a list to resolve. An empty format sniffs one
type Request struct {
	Text   string `json:"text"`
	Format string `json:"format"`
}

// Started is what Resolve answers with at once: the job streaming each resolved
// row, the detected format, and every parsed row, so the table can draw every
// line before any lookup returns
type Started struct {
	JobID  string         `json:"jobId"`
	Format string         `json:"format"`
	Rows   []cardlist.Row `json:"rows"`
}

// Resolve parses a list and starts resolving its rows. Starting a resolve
// abandons any earlier one still running
func (s *Service) Resolve(req Request) (Started, error) {
	rows, format, err := cardlist.Parse(req.Text, req.Format)
	if err != nil {
		return Started{}, apierr.Wrap(apierr.BadRequest, "", err)
	}
	if rows == nil {
		rows = []cardlist.Row{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.resolveCancel != nil {
		s.resolveCancel()
	}
	s.resolveCancel = cancel
	s.mu.Unlock()

	j := s.ws.Jobs.New()
	go func() {
		defer cancel()
		s.resolveRows(ctx, j, s.resolver(), rows)
	}()
	return Started{JobID: j.ID(), Format: format, Rows: rows}, nil
}

// resolveRows looks every row up and emits each result as it lands, in
// completion order with its index, then a final done event. A cancelled resolve
// ends without a done event's error, since the page has already moved on
func (s *Service) resolveRows(ctx context.Context, j *jobs.Job, rv cardlist.Resolver, rows []cardlist.Row) {
	err := cardlist.ResolveAll(ctx, rv, &s.resolved, rows, func(n int, res cardlist.Resolved) {
		j.Emit(jobs.Event{
			Step: fmt.Sprintf("Resolved %d of %d", n, len(rows)),
			Frac: float64(n) / float64(len(rows)),
			Row:  &res,
		})
	})
	if err != nil {
		j.Emit(jobs.Event{Done: true, Err: "resolve superseded"})
		return
	}
	j.Emit(jobs.Event{Done: true, Frac: 1, Step: fmt.Sprintf("Resolved %d of %d", len(rows), len(rows))})
}

// resolver picks the lookup source for a new resolve. The local copy is used
// when it is chosen in settings and loaded, and the API otherwise
func (s *Service) resolver() cardlist.Resolver {
	if s.ws.UseLocalCards() {
		return cardlist.LocalResolver(s.ws.Cards, s.ws.Pipe.Client())
	}
	return cardlist.APIResolver(s.ws.Pipe.Client())
}

// ReadFile returns the text of a list file the user opened or dropped
func (s *Service) ReadFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", apierr.Wrap(apierr.BadRequest, "opening the list", err)
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, maxListBytes+1))
	if err != nil {
		return "", apierr.Wrap(apierr.BadRequest, "reading the list", err)
	}
	if len(buf) > maxListBytes {
		return "", apierr.Newf(apierr.TooLarge, "the list must be under %d MB", maxListBytes>>20)
	}
	return string(buf), nil
}
