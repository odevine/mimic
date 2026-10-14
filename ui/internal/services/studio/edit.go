package studio

import (
	"encoding/json"
	"errors"
	"image"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
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
	Kind   string   `json:"kind"`
	Key    string   `json:"key"`
	Part   string   `json:"part,omitempty"`
	After  string   `json:"after,omitempty"`
	Images []string `json:"images,omitempty"`
	Drawn  bool     `json:"drawn"`
	Why    string   `json:"why,omitempty"`
	// Ink says the places are where the item's text is drawn and not the box's own
	// rectangle, which a text box set on its baseline has no use for. Such an item
	// can be moved and not resized
	Ink       bool     `json:"ink,omitempty"`
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
			Drawn: it.Drawn, Why: it.Why, Ink: it.Ink, Authored: rectView(it.Authored), Delivered: rectView(it.Delivered),
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

// ApplyResult is the draft's revision after the edits, the operations that undo them,
// and what is found in the draft as it now is
type ApplyResult struct {
	Rev     int            `json:"rev"`
	Inverse []draftedit.Op `json:"inverse"`
	// Valid says whether the draft loads now, and Diagnostics lists what is found in
	// it, the errors and the warnings
	Valid       bool             `json:"valid"`
	Diagnostics []DiagnosticView `json:"diagnostics"`
}

// Apply makes the edits to the draft's files and saves them at once. A batch that
// cannot be made is refused with the reason and changes nothing, and so is one that
// would leave a draft that loaded unable to load. The JSON editor's whole-file
// writes are let through when the draft did not load to begin with, since a draft in
// that state is for the JSON to fix
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
	wholeFile := req.Ops[0].Op == draftedit.OpReplace
	before, _ := inspect(fsys)
	after, ds := inspect(overlayFS{base: fsys, over: res.Files})
	if before && !after && !wholeFile {
		return ApplyResult{}, apierr.Newf(apierr.BadRequest, "That is not allowed: %s", firstProblem(ds, nil))
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
	return ApplyResult{Rev: rev, Inverse: res.Inverse, Valid: after, Diagnostics: diagnosticViews(ds)}, nil
}

// inspect reports whether a folder is a template that loads, and what is found in it.
// It reads no image, so a draft with large frames is checked in a moment
func inspect(fsys fs.FS) (bool, manifest.Diagnostics) {
	t, ds, err := author.NewDraft(fsys, 0).Open(author.Options{})
	return err == nil && t != nil, ds
}

// FileView is one of the files a draft's template is written in
type FileView struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// SourceView is the files of a draft that can be edited as JSON
type SourceView struct {
	Files []FileView `json:"files"`
}

// Source reads template.json and each layout as they are on disk, for the JSON editor
func (s *Service) Source(id string) (SourceView, error) {
	fsys, err := s.store.FS(id)
	if err != nil {
		return SourceView{}, err
	}
	names := []string{"template.json"}
	entries, err := fs.ReadDir(fsys, "layouts")
	if err == nil {
		var layouts []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				layouts = append(layouts, path.Join("layouts", e.Name()))
			}
		}
		sort.Strings(layouts)
		names = append(names, layouts...)
	}
	out := SourceView{Files: make([]FileView, 0, len(names))}
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return SourceView{}, apierr.Wrap(apierr.NotFound, "Could not read "+name, err)
		}
		if len(raw) > manifest.MaxBytes {
			return SourceView{}, apierr.Newf(apierr.TooLarge, "%s is over the %d bytes a template file may be", name, manifest.MaxBytes)
		}
		out.Files = append(out.Files, FileView{Name: name, Content: string(raw)})
	}
	return out, nil
}

// Problem is one finding about the text of a file. Line and Column say where in the
// text for a mistake in the JSON itself, and are zero for one about what it means,
// which Path locates by the keys instead
type Problem struct {
	Severity string `json:"severity"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

// CheckRequest is the text a file would hold
type CheckRequest struct {
	ID      string `json:"id"`
	File    string `json:"file"`
	Content string `json:"content"`
}

// CheckResult is what is found in the draft with the text in the file. Writable says
// whether the text can be saved, which it can when it is JSON and the draft would
// load, or did not load already. Nothing is written
type CheckResult struct {
	Writable bool      `json:"writable"`
	Problems []Problem `json:"problems"`
}

// CheckFile reports what is wrong with the text of a file as it is typed
func (s *Service) CheckFile(req CheckRequest) (CheckResult, error) {
	fsys, err := s.store.FS(req.ID)
	if err != nil {
		return CheckResult{}, err
	}
	res, err := draftedit.Apply(fsys, []draftedit.Op{{Op: draftedit.OpReplace, File: req.File, Content: req.Content}})
	if err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			line, col := position(req.Content, syntax.Offset)
			return CheckResult{Problems: []Problem{{Severity: "error", Line: line, Column: col, Message: syntax.Error()}}}, nil
		}
		return CheckResult{Problems: []Problem{{Severity: "error", Message: err.Error()}}}, nil
	}
	before, _ := inspect(fsys)
	after, ds := inspect(overlayFS{base: fsys, over: res.Files})
	out := CheckResult{Writable: after || !before, Problems: []Problem{}}
	for _, d := range ds {
		out.Problems = append(out.Problems, Problem{Severity: d.Severity.String(), Path: d.Path, Message: d.Message})
	}
	return out, nil
}

// position turns a byte offset in text into a line and a column, both counted from 1
func position(text string, offset int64) (line, col int) {
	if offset > int64(len(text)) {
		offset = int64(len(text))
	}
	line, col = 1, 1
	for _, r := range text[:max(offset, 0)] {
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return line, col
}

// VocabView lists the names a template's fields can take, which the editor offers as
// choices
type VocabView struct {
	// Draws is what a text box can be filled with, Conditions the facts about a card a
	// layer or box can depend on, FontRoles the text roles every template has,
	// BlendModes the ways a layer composites and Slots what a part can read its image
	// from
	Draws      []string `json:"draws"`
	Conditions []string `json:"conditions"`
	FontRoles  []string `json:"fontRoles"`
	BlendModes []string `json:"blendModes"`
	Slots      []string `json:"slots"`
}

// blendModes are the compositing modes a manifest can name, as the engine's list has
// them
var blendModes = []string{"normal", "multiply", "screen", "overlay", "softlight", "hardlight", "colordodge", "colorburn", "darken", "lighten", "difference", "exclusion", "hue", "saturation", "color", "luminosity"}

// Vocabulary lists the names the editor offers as choices
func (s *Service) Vocabulary() VocabView {
	return VocabView{
		Draws:      card.TextBoxNames(),
		Conditions: frame.Conditions(),
		FontRoles:  []string{string(fonts.Title), string(fonts.Body), string(fonts.BodyItalic), string(fonts.Mana), string(fonts.SmallCaps), string(fonts.Info)},
		BlendModes: blendModes,
		Slots:      frame.SlotNames(),
	}
}
