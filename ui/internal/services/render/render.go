// Package render is the service behind the single card preview and Save
package render

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/progress"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// Service renders one card at a time and keeps the finished image for Save
type Service struct{ ws *workspace.Workspace }

// New returns the service over a workspace
func New(ws *workspace.Workspace) *Service { return &Service{ws: ws} }

// Request is a render: the selected card as fetched, plus the editor's current
// field values overlaid onto it. Sending the base card keeps fields the form
// does not expose (color identity, produced mana, artwork URL) intact and lets
// the art cache key on the unchanged URL
type Request struct {
	Base  card.Data      `json:"base"`
	Edits cardlist.Edits `json:"edits"`
	// Target picks which resolution to render at: "output" for the full-size
	// render behind Save, anything else for the preview
	Target string `json:"target,omitempty"`
	// Face picks which of the card's images to render, 0 for the front and 1
	// for a double-faced card's back. The edits apply to the front face
	Face int `json:"face,omitempty"`
}

// Started is what Start answers with: the job to watch and the resolution the
// render uses
type Started struct {
	JobID string `json:"jobId"`
	DPI   int    `json:"dpi"`
}

// Start begins a render and returns its job. The render runs in a goroutine that
// emits progress, and the finished image is read with Image or written with Save
func (s *Service) Start(req Request) (Started, error) {
	d := cardlist.ApplyEdits(&req.Base, req.Edits)
	dpi := s.ws.ClampedDPI(req.Target)

	j := s.ws.Jobs.New()
	ctx, cancel := context.WithCancel(context.Background())
	j.SetCancel(cancel)
	go s.do(ctx, j, d, req.Face, dpi)
	return Started{JobID: j.ID(), DPI: dpi}, nil
}

// do fetches the face's art (once per URL) and renders that face through the
// template chosen for it at dpi, emitting progress and ending with a done event
// carrying whether art was missing
func (s *Service) do(parent context.Context, j *jobs.Job, d *card.Data, face, dpi int) {
	defer j.Cancel()
	artCtx, cancelArt := context.WithTimeout(parent, workspace.NetTimeout)
	art, artErr := s.ws.ArtFor(artCtx, d.Face(face))
	cancelArt()

	// The render is local work on a bound that has nothing to do with the
	// network's, and a full-resolution export is the slowest thing the app does
	ctx, cancel := context.WithTimeout(parent, workspace.RenderTimeout)
	defer cancel()

	img, err := s.ws.Pipe.Render(ctx, d, face, art, dpi, progress.Render(func(step string, frac float64) {
		j.Emit(jobs.Event{Step: step, Frac: frac})
	}))
	if err != nil {
		j.Emit(jobs.Event{Done: true, Err: err.Error()})
		return
	}
	j.SetResult(img, pipeline.FaceName(d, face))
	j.Emit(jobs.Event{Done: true, ArtMissing: artErr != nil})
}

// Cancel stops a render the client no longer wants, such as a preview superseded
// by a newer edit. Cancelling a finished job does nothing
func (s *Service) Cancel(jobID string) error {
	j, ok := s.ws.Jobs.Lookup(jobID)
	if !ok {
		return apierr.New(apierr.NotFound, "no such render")
	}
	j.Cancel()
	return nil
}

// Image returns a finished render and the card's name, for the image route
func (s *Service) Image(jobID string) (image.Image, string, error) {
	j, ok := s.ws.Jobs.Lookup(jobID)
	if !ok {
		return nil, "", apierr.New(apierr.NotFound, "no such render")
	}
	img, name := j.Result()
	if img == nil {
		return nil, "", apierr.New(apierr.NotFound, "render not finished")
	}
	return img, name, nil
}

// Save writes a finished render to path as a PNG
func (s *Service) Save(jobID, path string) error {
	img, _, err := s.Image(jobID)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return apierr.Wrap(apierr.BadRequest, "saving the image", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return apierr.Wrap(apierr.Internal, "encoding image", err)
	}
	if err := f.Close(); err != nil {
		return apierr.Wrap(apierr.Internal, "saving the image", err)
	}
	return nil
}

// Filename turns a card name into a safe .png filename
func Filename(name string) string {
	if name == "" {
		return "card.png"
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, name)
	return fmt.Sprintf("%s.png", cleaned)
}
