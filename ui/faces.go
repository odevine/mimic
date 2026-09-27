package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sort"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/prefs"
)

// shapeKey is how a shape is keyed in prefs and on the wire, as in
// "transform_front/standard"
func shapeKey(sh template.Shape) string { return string(sh.Role) + "/" + string(sh.Kind) }

// primaryShape is the shape the top-bar template picker chooses for. Every
// other shape has its own preference in settings
var primaryShape = template.Shape{Role: template.RoleSingle, Kind: template.KindStandard}

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
	if looseDir(name) != "" {
		return true
	}
	_, ok := newestCachedVersion(name)
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

// choose decides which template renders a face of shape sh, without loading
// it: the face's own preference when that template is installed and supports
// it, then the active template when it supports it, then the first installed
// template that does. It reports false when none does
func (p *renderPipeline) choose(sh template.Shape) (prefs.TemplateChoice, bool) {
	if sh != primaryShape && p.preferences != nil {
		if c, ok := p.preferences()[shapeKey(sh)]; ok && supportsOf(c.Name).Allows(sh) && installed(c.Name) {
			return c, true
		}
	}
	if at := p.active.Load(); supportsOf(at.name).Allows(sh) {
		return prefs.TemplateChoice{Name: at.name, Version: at.version}, true
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
func (p *renderPipeline) templateFor(sh template.Shape) (*activeTemplate, error) {
	c, ok := p.choose(sh)
	if !ok {
		return nil, &noTemplateError{Shape: sh}
	}
	return p.load(c)
}

// load returns the template c names, reusing the active one or one loaded
// earlier. A template is loaded once per name through the same network-free
// chain as the startup template, or from the exact version c names when that
// is installed, and stays open until shutdown
func (p *renderPipeline) load(c prefs.TemplateChoice) (*activeTemplate, error) {
	matches := func(at *activeTemplate) bool {
		return at != nil && at.name == c.Name && (c.Version == "" || at.version == c.Version)
	}
	if at := p.active.Load(); matches(at) {
		return at, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if at := p.loaded[c.Name]; matches(at) {
		return at, nil
	}
	var at *activeTemplate
	var err error
	if c.Version != "" && (isVersionCached(c.Name, c.Version) || (c.Version == localVersion && looseDir(c.Name) != "")) {
		at, err = activeFromVersion(context.Background(), c.Name, c.Version, nil)
	} else {
		at, _, err = resolveActiveTemplate(c.Name)
	}
	if err != nil {
		return nil, fmt.Errorf("loading template %s: %w", c.Name, err)
	}
	if p.loaded == nil {
		p.loaded = map[string]*activeTemplate{}
	}
	if old := p.loaded[c.Name]; old != nil {
		log.Printf("mimic: template %s %s replaced by %s", old.name, old.version, at.version)
	}
	p.loaded[c.Name] = at
	if at.cleanup != nil {
		p.cleanups = append(p.cleanups, at.cleanup)
	}
	return at, nil
}

// unsupported reports why face of d cannot render, or nil when some template
// renders it. A run checks this before fetching any art, the same check the
// engine makes again at render time
func (p *renderPipeline) unsupported(d *card.Data, face int) error {
	shapes := template.Classify(d)
	if face < 0 || face >= len(shapes) {
		return fmt.Errorf("face %d out of range, %q renders %d", face, d.Name, len(shapes))
	}
	if _, ok := p.choose(shapes[face]); !ok {
		return &noTemplateError{Shape: shapes[face]}
	}
	return nil
}

// isUnsupported reports whether err is a face no template renders, as opposed
// to a render that failed partway
func isUnsupported(err error) bool {
	var u *template.UnsupportedError
	var n *noTemplateError
	return errors.As(err, &u) || errors.As(err, &n)
}

// faceTemplates maps every shape some installed or active template renders to
// the name of the template that renders it, keyed by shapeKey. The page checks
// a card's shapes against it
func (p *renderPipeline) faceTemplates() map[string]string {
	regs := installedTemplates()
	if at := p.active.Load(); !installed(at.name) {
		regs = append(regs, template.Registration{Name: at.name, Supports: supportsOf(at.name)})
	}
	out := map[string]string{}
	for _, sh := range shapesOf(regs) {
		if c, ok := p.choose(sh); ok {
			out[shapeKey(sh)] = c.Name
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
				seen[shapeKey(sh)] = sh
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

// faceName is the name a face prints, which names its output file
func faceName(d *card.Data, face int) string {
	return d.Face(face).Name
}

// faceOption is one installed template a face shape can render through, with
// the versions of it that need no download
type faceOption struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

// faceRow is one face shape in the settings list: the installed templates
// that render it, the saved preference if any, and the template in use now
type faceRow struct {
	Key     string                `json:"key"`
	Role    template.Role         `json:"role"`
	Kind    template.Kind         `json:"kind"`
	Options []faceOption          `json:"options"`
	Chosen  *prefs.TemplateChoice `json:"chosen,omitempty"`
	Using   string                `json:"using"`
	// Primary marks the standard single card row, whose choice is the active
	// template the top bar shows rather than a saved preference
	Primary bool `json:"primary,omitempty"`
}

// faceRows lists the standard single card first, chosen by the active
// template, then each other face shape some installed template renders, so
// the settings list stays as short as what is installed
func (p *renderPipeline) faceRows() []faceRow {
	regs := installedTemplates()
	var saved map[string]prefs.TemplateChoice
	if p.preferences != nil {
		saved = p.preferences()
	}
	active := p.active.Load()
	primary := faceRow{
		Key: shapeKey(primaryShape), Role: primaryShape.Role, Kind: primaryShape.Kind, Primary: true,
		Options: faceOptions(regs, primaryShape),
		Chosen:  &prefs.TemplateChoice{Name: active.name, Version: active.version},
		Using:   templateDisplay(active.name, active.version),
	}
	rows := []faceRow{primary}
	for _, sh := range shapesOf(regs) {
		if sh == primaryShape {
			continue
		}
		row := faceRow{Key: shapeKey(sh), Role: sh.Role, Kind: sh.Kind, Options: faceOptions(regs, sh)}
		if c, ok := saved[row.Key]; ok {
			row.Chosen = &c
		}
		if c, ok := p.choose(sh); ok {
			// An empty version is the newest installed, which loads the way
			// resolveActiveTemplate does, a local folder first
			if v := installedVersions(c.Name); c.Version == "" && len(v) > 0 {
				c.Version = v[0]
			}
			row.Using = templateDisplay(c.Name, c.Version)
		}
		rows = append(rows, row)
	}
	return rows
}

// installedVersions lists the versions of a template that render without a
// download: the local developer version first, then cached bundles, newest
// first
func installedVersions(name string) []string {
	var out []string
	if looseDir(name) != "" {
		out = append(out, localVersion)
	}
	cached, _ := cachedVersions(name)
	return append(out, cached...)
}

func (s *server) handleFaceTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.pipe.faceRows())
}

// faceChoiceBody is the PUT /api/template/faces payload. An empty Name clears
// the shape's preference
type faceChoiceBody struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// handleSetFaceTemplate saves the preferred template for one face shape, or
// for standard single cards switches the active template, the same choice the
// top bar makes. It waits for a run to finish, so every face of a run renders
// through the templates it started with
func (s *server) handleSetFaceTemplate(w http.ResponseWriter, r *http.Request) {
	var body faceChoiceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad face template request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if s.runActive() {
		http.Error(w, "face templates cannot change while a run is in progress", http.StatusConflict)
		return
	}
	var row *faceRow
	for _, fr := range s.pipe.faceRows() {
		if fr.Key == body.Key {
			row = &fr
			break
		}
	}
	if row == nil {
		http.Error(w, "no installed template renders "+body.Key, http.StatusBadRequest)
		return
	}
	if body.Name != "" && !slices.ContainsFunc(row.Options, func(o faceOption) bool {
		return o.Name == body.Name && (body.Version == "" || slices.Contains(o.Versions, body.Version))
	}) {
		http.Error(w, fmt.Sprintf("%s %s is not an installed template for %s", body.Name, body.Version, body.Key), http.StatusBadRequest)
		return
	}
	if row.Primary {
		if err := s.switchActive(body.Name, body.Version); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, s.pipe.faceRows())
		return
	}
	s.prefs.SetFaceTemplate(body.Key, prefs.TemplateChoice{Name: body.Name, Version: body.Version})
	writeJSON(w, s.pipe.faceRows())
}

// faceOptions lists the installed templates that render sh, with their
// installed versions
func faceOptions(regs []template.Registration, sh template.Shape) []faceOption {
	var out []faceOption
	for _, r := range regs {
		if r.Supports.Allows(sh) {
			out = append(out, faceOption{Name: r.Name, Versions: installedVersions(r.Name)})
		}
	}
	return out
}

// switchActive makes an installed template the active one, as choosing it in
// the top bar would. An empty version is the newest installed. It never
// downloads, since the settings list offers only installed versions
func (s *server) switchActive(name, version string) error {
	if name == "" {
		return fmt.Errorf("standard cards need a template")
	}
	if version == "" {
		v := installedVersions(name)
		if len(v) == 0 {
			return fmt.Errorf("%s is not installed", name)
		}
		version = v[0]
	}
	at, err := activeFromVersion(context.Background(), name, version, nil)
	if err != nil {
		return err
	}
	s.setActiveTemplate(at)
	return nil
}
