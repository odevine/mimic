// Package studio is the service behind the template editor. It makes, copies and
// removes drafts, opens one to read what it holds, remembers the card it is tested
// with, and renders that card through the draft as it is on disk
package studio

import (
	"context"
	"image"
	"sort"
	"sync"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/drafts"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/progress"
	"github.com/odevine/mimic/ui/internal/services/render"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// imageBudget is the decoded image data a draft keeps while it is open, which is
// what makes a render after an edit cost one warm render and not a cold one
const imageBudget = template.DefaultImageCacheBytes

// Service is the editor's drafts and the templates opened from them
type Service struct {
	ws    *workspace.Workspace
	store *drafts.Store

	mu   sync.Mutex
	open map[string]*author.Draft
	// revs counts the edits made to each draft, and editMu keeps two batches of
	// edits from reading the same files and each writing over the other
	revs   map[string]int
	editMu sync.Mutex
	// held is the files edits wrote and removed, which an undo writes back
	held   *stash
	thumbs thumbCache
}

// New returns the service over a workspace, keeping drafts in the app's drafts
// folder
func New(ws *workspace.Workspace) *Service {
	return NewWith(ws, drafts.NewStore(workspace.DraftsDir()))
}

// NewWith returns the service over a workspace and the store of its drafts
func NewWith(ws *workspace.Workspace, store *drafts.Store) *Service {
	return &Service{ws: ws, store: store, open: map[string]*author.Draft{}, revs: map[string]int{}, held: newStash(stashBytes)}
}

// Shutdown releases the images every open draft holds
func (s *Service) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, d := range s.open {
		d.Close()
		delete(s.open, id)
	}
}

// List returns the drafts, newest first
func (s *Service) List() ([]drafts.Summary, error) { return s.store.List() }

// CreateRequest names a new blank template and the resolution its canvas is made at,
// where zero is the default
type CreateRequest struct {
	Name string `json:"name"`
	DPI  int    `json:"dpi"`
}

// Create makes a draft from the starter
func (s *Service) Create(req CreateRequest) (drafts.Summary, error) {
	return s.store.Create(req.Name, req.DPI)
}

// ForkRequest names an installed template version to copy, and the copy's name
type ForkRequest struct {
	Template string `json:"template"`
	Version  string `json:"version"`
	Name     string `json:"name"`
}

// Fork makes a draft from a copy of an installed template version
func (s *Service) Fork(req ForkRequest) (drafts.Summary, error) {
	p, header, closeFn, err := pipeline.OpenSource(req.Template, req.Version)
	if err != nil {
		return drafts.Summary{}, apierr.Wrap(apierr.NotFound, "That template is not installed", err)
	}
	defer closeFn()
	return s.store.Fork(p, header, req.Name)
}

// RenameRequest gives a draft another name
type RenameRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Rename changes the name a draft shows
func (s *Service) Rename(req RenameRequest) (drafts.Summary, error) {
	return s.store.Rename(req.ID, req.Name)
}

// Delete removes a draft
func (s *Service) Delete(id string) error {
	if err := s.store.Delete(id); err != nil {
		return err
	}
	s.forget(id)
	return nil
}

// forget closes the images a draft holds
func (s *Service) forget(id string) {
	s.mu.Lock()
	d := s.open[id]
	delete(s.open, id)
	s.mu.Unlock()
	if d != nil {
		d.Close()
	}
}

// handle is the draft's opener, made on first use and kept so its images are shared
// by every render
func (s *Service) handle(id string) (*author.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.open[id]; ok {
		return d, nil
	}
	fsys, err := s.store.FS(id)
	if err != nil {
		return nil, err
	}
	d := author.NewDraft(fsys, imageBudget)
	s.open[id] = d
	return d, nil
}

// DiagnosticView is one finding about a draft, with the path to it in the files
type DiagnosticView struct {
	Severity string `json:"severity"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

func diagnosticViews(ds manifest.Diagnostics) []DiagnosticView {
	out := make([]DiagnosticView, len(ds))
	for i, d := range ds {
		out[i] = DiagnosticView{Severity: d.Severity.String(), Path: d.Path, Message: d.Message}
	}
	return out
}

// LayoutView is one layout of a draft: the canvas it is drawn on and the faces it
// renders
type LayoutView struct {
	Name   string           `json:"name"`
	Width  int              `json:"width"`
	Height int              `json:"height"`
	DPI    int              `json:"dpi"`
	Rotate int              `json:"rotate,omitempty"`
	Serves []template.Shape `json:"serves"`
}

// OpenView is a draft as the editor opens it
type OpenView struct {
	Draft drafts.Summary `json:"draft"`
	// Meta is the card the draft was last tested with
	Meta drafts.Meta `json:"meta"`
	// Layouts is empty when the draft does not load, and Diagnostics says why
	Layouts     []LayoutView     `json:"layouts"`
	Diagnostics []DiagnosticView `json:"diagnostics"`
	// Valid is whether the draft loads and can render
	Valid bool `json:"valid"`
}

// Open reads a draft as it is now, so a change made outside the app is seen. It
// starts the draft's images afresh
func (s *Service) Open(id string) (OpenView, error) {
	sum, err := s.store.Get(id)
	if err != nil {
		return OpenView{}, err
	}
	s.forget(id)
	d, err := s.handle(id)
	if err != nil {
		return OpenView{}, err
	}
	meta, err := s.store.Meta(id)
	if err != nil {
		return OpenView{}, err
	}
	view := OpenView{Draft: sum, Meta: meta, Layouts: []LayoutView{}, Diagnostics: []DiagnosticView{}}
	t, ds, err := d.Open(author.Options{})
	view.Diagnostics = diagnosticViews(ds)
	if err != nil && t == nil {
		if len(ds) == 0 {
			view.Diagnostics = []DiagnosticView{{Severity: "error", Message: err.Error()}}
		}
		return view, nil
	}
	view.Valid = true
	view.Layouts = layoutViews(t)
	return view, nil
}

// layoutViews describes the layouts of a template, sorted by name
func layoutViews(t *template.Template) []LayoutView {
	m := t.Manifest()
	names := make([]string, 0, len(m.Layouts))
	for name := range m.Layouts {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]LayoutView, 0, len(names))
	for _, name := range names {
		ml := m.Layouts[name]
		v := LayoutView{Name: name, Width: ml.Canvas.Width, Height: ml.Canvas.Height, DPI: ml.Canvas.DPI, Rotate: ml.Canvas.Rotate, Serves: ml.Serves}
		if len(ml.Serves) > 0 {
			if l, ok := t.Layout(ml.Serves[0]); ok {
				v.DPI = l.NativeDPI()
			}
		}
		out = append(out, v)
	}
	return out
}

// TestCardRequest is the card a draft is tested with and which of its faces
type TestCardRequest struct {
	ID   string     `json:"id"`
	Card *card.Data `json:"card"`
	Face int        `json:"face"`
}

// SetTestCard remembers the card a draft is tested with
func (s *Service) SetTestCard(req TestCardRequest) error {
	meta, err := s.store.Meta(req.ID)
	if err != nil {
		return err
	}
	meta.TestCard, meta.Face = req.Card, req.Face
	return s.store.SaveMeta(req.ID, meta)
}

// RenderRequest is a render of a card through a draft. Face picks which of the
// card's images, 0 for the front
type RenderRequest struct {
	ID   string    `json:"id"`
	Card card.Data `json:"card"`
	Face int       `json:"face"`
}

// Render draws the card through the draft as it is on disk, at the preview
// resolution, and returns the job to watch. Its image is read with the same route
// as any render. A draft that does not load, or does not render the card, is
// refused here with the reason
func (s *Service) Render(req RenderRequest) (render.Started, error) {
	d, err := s.handle(req.ID)
	if err != nil {
		return render.Started{}, err
	}
	t, ds, err := d.Open(author.Options{})
	if err != nil || t == nil {
		return render.Started{}, apierr.New(apierr.BadRequest, firstProblem(ds, err))
	}
	shapes := template.Classify(&req.Card)
	if req.Face < 0 || req.Face >= len(shapes) {
		return render.Started{}, apierr.Newf(apierr.BadRequest, "%s has no face %d", req.Card.Name, req.Face)
	}
	layout, ok := t.Layout(shapes[req.Face])
	if !ok {
		return render.Started{}, apierr.Newf(apierr.BadRequest, "This template does not render %s", shapes[req.Face])
	}
	dpi := layout.ClampDPI(s.ws.RenderDPI(workspace.TargetPreview))
	at := pipeline.NewDraftTemplate(t.ID(), t)

	j := s.ws.Jobs.New()
	ctx, cancel := context.WithCancel(context.Background())
	j.SetCancel(cancel)
	go s.do(ctx, j, at, &req.Card, req.Face, dpi)
	return render.Started{JobID: j.ID(), DPI: dpi}, nil
}

// firstProblem is the first error a draft reports, or the error that stopped it
func firstProblem(ds manifest.Diagnostics, err error) string {
	for _, d := range ds {
		if d.Severity == manifest.Error {
			return d.String()
		}
	}
	if err != nil {
		return err.Error()
	}
	return "The template did not load"
}

func (s *Service) do(parent context.Context, j *jobs.Job, at *pipeline.Template, d *card.Data, face, dpi int) {
	defer j.Cancel()
	artCtx, cancelArt := context.WithTimeout(parent, workspace.NetTimeout)
	art, artErr := s.ws.ArtFor(artCtx, d.Face(face))
	cancelArt()

	ctx, cancel := context.WithTimeout(parent, workspace.RenderTimeout)
	defer cancel()
	var img image.Image
	img, err := s.ws.Pipe.RenderThrough(ctx, at, d, face, art, dpi, progress.Render(func(step string, frac float64) {
		j.Emit(jobs.Event{Step: step, Frac: frac})
	}))
	if err != nil {
		j.Emit(jobs.Event{Done: true, Err: err.Error()})
		return
	}
	j.SetResult(img, pipeline.FaceName(d, face))
	j.Emit(jobs.Event{Done: true, ArtMissing: artErr != nil})
}
