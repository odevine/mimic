package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/prefs"
)

// ShapeKey is how a shape is keyed in prefs and on the wire, as in
// "transform_front/standard"
func ShapeKey(sh template.Shape) string { return string(sh.Role) + "/" + string(sh.Kind) }

// PrimaryShape is the shape the top-bar template picker chooses for. Every
// other shape has its own preference in settings
var PrimaryShape = template.Shape{Role: template.RoleSingle, Kind: template.KindStandard}

// noTemplateError is a face no installed template renders. It settles a run's
// card as unsupported, the same as an *template.UnsupportedError
type noTemplateError struct {
	Shape template.Shape
}

func (e *noTemplateError) Error() string {
	return fmt.Sprintf("no installed template renders %s faces", e.Shape)
}

// installed reports whether a template can render without a download: it has
// a loose developer folder or a cached bundle. Placeholders do not count, since
// they would render a face as flat boxes
func installed(name string) bool {
	if LooseDir(name) != "" {
		return true
	}
	_, ok := catalog.NewestCachedVersion(name)
	return ok
}

// installedTemplates lists the registered templates that are installed, sorted
// by name
func installedTemplates() []template.Registration {
	var out []template.Registration
	for _, r := range template.List() {
		if installed(r.Name) {
			out = append(out, r)
		}
	}
	return out
}

// Choose decides which template renders a face of shape sh, without loading
// it: the face's own preference when that template is installed and supports
// it, then the active template when it supports it, then the first installed
// template that does. It reports false when none does
func (p *Pipeline) Choose(sh template.Shape) (prefs.TemplateChoice, bool) {
	if sh != PrimaryShape && p.preferences != nil {
		if c, ok := p.preferences()[ShapeKey(sh)]; ok && SupportsOf(c.Name).Allows(sh) && installed(c.Name) {
			return c, true
		}
	}
	if at := p.active.Load(); SupportsOf(at.Name).Allows(sh) {
		return prefs.TemplateChoice{Name: at.Name, Version: at.Version}, true
	}
	for _, r := range installedTemplates() {
		if r.Supports.Allows(sh) {
			return prefs.TemplateChoice{Name: r.Name}, true
		}
	}
	return prefs.TemplateChoice{}, false
}

// templateFor loads the template that renders a face of shape sh, or returns a
// *noTemplateError when none does
func (p *Pipeline) templateFor(sh template.Shape) (*Template, error) {
	c, ok := p.Choose(sh)
	if !ok {
		return nil, &noTemplateError{Shape: sh}
	}
	return p.load(c)
}

// load returns the template c names, reusing the active one or one loaded
// earlier. A template is loaded once per name through the same network-free
// chain as the startup template, or from the exact version c names when that
// is installed, and stays open until shutdown
func (p *Pipeline) load(c prefs.TemplateChoice) (*Template, error) {
	matches := func(at *Template) bool {
		return at != nil && at.Name == c.Name && (c.Version == "" || at.Version == c.Version)
	}
	if at := p.active.Load(); matches(at) {
		return at, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if at := p.loaded[c.Name]; matches(at) {
		return at, nil
	}
	var at *Template
	var err error
	if c.Version != "" && (catalog.IsCached(c.Name, c.Version) || (c.Version == LocalVersion && LooseDir(c.Name) != "")) {
		at, err = FromVersion(context.Background(), c.Name, c.Version, nil)
	} else {
		at, _, err = Resolve(c.Name)
	}
	if err != nil {
		return nil, fmt.Errorf("loading template %s: %w", c.Name, err)
	}
	if p.loaded == nil {
		p.loaded = map[string]*Template{}
	}
	if old := p.loaded[c.Name]; old != nil {
		log.Printf("mimic: template %s %s replaced by %s", old.Name, old.Version, at.Version)
	}
	p.loaded[c.Name] = at
	if at.cleanup != nil {
		p.cleanups = append(p.cleanups, at.cleanup)
	}
	return at, nil
}

// Unsupported reports why face of d cannot render, or nil when some template
// renders it. A run checks this before fetching any art, the same check the
// engine makes again at render time
func (p *Pipeline) Unsupported(d *card.Data, face int) error {
	shapes := template.Classify(d)
	if face < 0 || face >= len(shapes) {
		return fmt.Errorf("face %d out of range, %q renders %d", face, d.Name, len(shapes))
	}
	if _, ok := p.Choose(shapes[face]); !ok {
		return &noTemplateError{Shape: shapes[face]}
	}
	return nil
}

// IsUnsupported reports whether err is a face no template renders, as opposed
// to a render that failed partway
func IsUnsupported(err error) bool {
	var u *template.UnsupportedError
	var n *noTemplateError
	return errors.As(err, &u) || errors.As(err, &n)
}

// FaceTemplates maps every shape some installed or active template renders to
// the name of the template that renders it, keyed by ShapeKey. The page checks
// a card's shapes against it
func (p *Pipeline) FaceTemplates() map[string]string {
	regs := installedTemplates()
	if at := p.active.Load(); !installed(at.Name) {
		regs = append(regs, template.Registration{Name: at.Name, Supports: SupportsOf(at.Name)})
	}
	out := map[string]string{}
	for _, sh := range shapesOf(regs) {
		if c, ok := p.Choose(sh); ok {
			out[ShapeKey(sh)] = c.Name
		}
	}
	return out
}

// shapesOf lists every shape the registrations support, deduplicated and
// sorted by key
func shapesOf(regs []template.Registration) []template.Shape {
	seen := map[string]template.Shape{}
	for _, r := range regs {
		for _, role := range r.Supports.Roles {
			for _, kind := range r.Supports.Kinds {
				sh := template.Shape{Role: role, Kind: kind}
				seen[ShapeKey(sh)] = sh
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]template.Shape, len(keys))
	for i, k := range keys {
		out[i] = seen[k]
	}
	return out
}

// FaceName is the name a face prints, which names its output file
func FaceName(d *card.Data, face int) string {
	return d.Face(face).Name
}

// FaceOption is one installed template a face shape can render through, with
// the versions of it that need no download
type FaceOption struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

// FaceRow is one face shape in the settings list: the installed templates
// that render it, the saved preference if any, and the template in use now
type FaceRow struct {
	Key     string                `json:"key"`
	Role    template.Role         `json:"role"`
	Kind    template.Kind         `json:"kind"`
	Options []FaceOption          `json:"options"`
	Chosen  *prefs.TemplateChoice `json:"chosen,omitempty"`
	Using   string                `json:"using"`
	// Primary marks the standard single card row, whose choice is the active
	// template the top bar shows rather than a saved preference
	Primary bool `json:"primary,omitempty"`
}

// FaceRows lists the standard single card first, chosen by the active
// template, then each other face shape some installed template renders, so
// the settings list stays as short as what is installed
func (p *Pipeline) FaceRows() []FaceRow {
	regs := installedTemplates()
	var saved map[string]prefs.TemplateChoice
	if p.preferences != nil {
		saved = p.preferences()
	}
	active := p.active.Load()
	primary := FaceRow{
		Key: ShapeKey(PrimaryShape), Role: PrimaryShape.Role, Kind: PrimaryShape.Kind, Primary: true,
		Options: faceOptions(regs, PrimaryShape),
		Chosen:  &prefs.TemplateChoice{Name: active.Name, Version: active.Version},
		Using:   Display(active.Name, active.Version),
	}
	rows := []FaceRow{primary}
	for _, sh := range shapesOf(regs) {
		if sh == PrimaryShape {
			continue
		}
		row := FaceRow{Key: ShapeKey(sh), Role: sh.Role, Kind: sh.Kind, Options: faceOptions(regs, sh)}
		if c, ok := saved[row.Key]; ok {
			row.Chosen = &c
		}
		if c, ok := p.Choose(sh); ok {
			// An empty version is the newest installed, which loads the way
			// Resolve does, a local folder first
			if v := InstalledVersions(c.Name); c.Version == "" && len(v) > 0 {
				c.Version = v[0]
			}
			row.Using = Display(c.Name, c.Version)
		}
		rows = append(rows, row)
	}
	return rows
}

// InstalledVersions lists the versions of a template that render without a
// download: the local developer version first, then cached bundles, newest
// first
func InstalledVersions(name string) []string {
	var out []string
	if LooseDir(name) != "" {
		out = append(out, LocalVersion)
	}
	cached, _ := catalog.CachedVersions(name)
	return append(out, cached...)
}

// faceOptions lists the installed templates that render sh, with their
// installed versions
func faceOptions(regs []template.Registration, sh template.Shape) []FaceOption {
	var out []FaceOption
	for _, r := range regs {
		if r.Supports.Allows(sh) {
			out = append(out, FaceOption{Name: r.Name, Versions: InstalledVersions(r.Name)})
		}
	}
	return out
}
