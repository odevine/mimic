package studio

import (
	"errors"
	"image"
	"io/fs"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/draftedit"
)

// RectView is a position and size in the finished image's pixels
type RectView struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func rectView(r image.Rectangle) RectView {
	return RectView{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy()}
}

// SizeView is the width and height of a canvas
type SizeView struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ItemView is one thing the layout draws for the test card, where it is, and why it
// does not draw when it does not. Authored is in the layout's own canvas and
// Delivered in the finished image, which differ when the layout turns the card. A
// layer that does not draw has no place
type ItemView struct {
	Kind      string   `json:"kind"`
	Key       string   `json:"key"`
	Part      string   `json:"part,omitempty"`
	After     string   `json:"after,omitempty"`
	Images    []string `json:"images,omitempty"`
	Drawn     bool     `json:"drawn"`
	Why       string   `json:"why,omitempty"`
	Authored  RectView `json:"authored"`
	Delivered RectView `json:"delivered"`
}

// DescribeRequest is the card a draft is being tested with
type DescribeRequest struct {
	ID   string    `json:"id"`
	Card card.Data `json:"card"`
	Face int       `json:"face"`
}

// DescribeView is what the layout the card renders with draws, in draw order
type DescribeView struct {
	Layout    string     `json:"layout"`
	Authored  SizeView   `json:"authored"`
	Delivered SizeView   `json:"delivered"`
	Items     []ItemView `json:"items"`
	// Editable is whether the layout's numbers can be changed from the editor, and
	// Reason says why not when they cannot
	Editable bool   `json:"editable"`
	Reason   string `json:"reason,omitempty"`
	// Rev counts the edits made to the draft since the app started, so an answer
	// about an older state can be told from the current one
	Rev int `json:"rev"`
}

// Describe lists what the draft's layout draws for the test card and where, which
// the editor draws its selection from
func (s *Service) Describe(req DescribeRequest) (DescribeView, error) {
	d, err := s.handle(req.ID)
	if err != nil {
		return DescribeView{}, err
	}
	t, ds, err := d.Open(author.Options{})
	if err != nil || t == nil {
		return DescribeView{}, apierr.New(apierr.BadRequest, firstProblem(ds, err))
	}
	r := template.RenderRequest{Card: &req.Card, Face: req.Face}
	// Whether art draws depends on the card having some, and its pixels do not matter
	if req.Card.Face(req.Face).ArtworkURL != "" {
		r.Art = image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	desc, err := render.Describe(t, r)
	if err != nil {
		return DescribeView{}, apierr.New(apierr.BadRequest, err.Error())
	}
	view := DescribeView{
		Layout:    desc.Layout,
		Authored:  SizeView{desc.Authored.X, desc.Authored.Y},
		Delivered: SizeView{desc.Delivered.X, desc.Delivered.Y},
		Items:     make([]ItemView, len(desc.Items)),
		Rev:       s.rev(req.ID),
	}
	for i, it := range desc.Items {
		view.Items[i] = ItemView{
			Kind: string(it.Kind), Key: it.Key, Part: it.Part, After: it.After, Images: it.Images,
			Drawn: it.Drawn, Why: it.Why, Authored: rectView(it.Authored), Delivered: rectView(it.Delivered),
		}
	}
	if fsys, err := s.store.FS(req.ID); err == nil {
		if err := draftedit.Editable(fsys, desc.Layout); err != nil {
			view.Reason = editReason(err)
		} else {
			view.Editable = true
		}
	}
	return view, nil
}

func editReason(err error) string {
	if errors.Is(err, draftedit.ErrExtends) {
		return "This layout extends another one, so its positions are not in its own file. Edit it in the JSON"
	}
	return err.Error()
}

func (s *Service) rev(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revs[id]
}

// ApplyRequest is a batch of edits to a draft, which are made together or not at all
type ApplyRequest struct {
	ID  string         `json:"id"`
	Ops []draftedit.Op `json:"ops"`
}

// ApplyResult is the draft's revision after the edits and the operations that undo
// them
type ApplyResult struct {
	Rev     int            `json:"rev"`
	Inverse []draftedit.Op `json:"inverse"`
}

// Apply makes the edits to the draft's files and saves them at once. A batch that
// cannot be made, or that would leave a draft that loaded no longer loading, is
// refused with the reason and changes nothing
func (s *Service) Apply(req ApplyRequest) (ApplyResult, error) {
	if len(req.Ops) == 0 {
		return ApplyResult{}, apierr.New(apierr.BadRequest, "There is nothing to change")
	}
	s.editMu.Lock()
	defer s.editMu.Unlock()
	fsys, err := s.store.FS(req.ID)
	if err != nil {
		return ApplyResult{}, err
	}
	res, err := draftedit.Apply(fsys, req.Ops)
	if err != nil {
		if errors.Is(err, draftedit.ErrExtends) {
			return ApplyResult{}, apierr.New(apierr.BadRequest, editReason(err))
		}
		return ApplyResult{}, apierr.Wrap(apierr.BadRequest, "That change cannot be made", err)
	}
	// What the draft would be with the change must still load, unless it did not
	// before, since a draft already broken is for the JSON to fix
	if loads(fsys) && !loads(overlayFS{base: fsys, over: res.Files}) {
		return ApplyResult{}, apierr.New(apierr.BadRequest, "That change would leave the template unable to load")
	}
	for name, data := range res.Files {
		if err := s.store.WriteFile(req.ID, name, data); err != nil {
			return ApplyResult{}, err
		}
	}
	s.mu.Lock()
	s.revs[req.ID]++
	rev := s.revs[req.ID]
	s.mu.Unlock()
	return ApplyResult{Rev: rev, Inverse: res.Inverse}, nil
}

// loads reports whether a folder is a template that loads. It reads no image, so a
// draft with large frames is checked in a moment
func loads(fsys fs.FS) bool {
	t, _, err := author.NewDraft(fsys, 0).Open(author.Options{})
	return err == nil && t != nil
}
