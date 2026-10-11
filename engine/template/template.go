// Package template defines what a loaded card template is. A template is read
// from an asset provider, which holds its resolved manifest and its images, and
// compiled once into layouts a render reads without parsing anything. A library
// holds the templates a program has loaded, and adds, replaces, and removes them
// while it runs
package template

import (
	"fmt"
	"image"
	"io"
	"sort"
	"sync"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

// ProgressFunc reports render progress: a human-readable step and a fraction in
// [0,1]. It may be called many times per step, and is called from the render
// goroutine, so an implementation that touches UI state marshals it itself
type ProgressFunc func(step string, frac float64)

// RenderRequest is everything a render needs to know about one card, beside the
// template it is drawn with
type RenderRequest struct {
	Card *card.Data
	// Face picks which of Classify(Card)'s images to render, 0 for the front.
	// Only a double-faced card has more than one
	Face int
	Art  image.Image // nil renders without art
	// SetSymbol is the card's set expansion symbol, nil to render without one
	SetSymbol *card.SetSymbol
	// Features holds the value chosen for each of the template's features
	Features frame.Features
	// Progress, when set, receives step updates during the render. Nil disables
	// reporting, which is the common case for a batch or a headless render
	Progress ProgressFunc
	// FontDir is an optional directory of user-supplied font overrides, one
	// file per role subfolder (title, body, body-italic, mana, type, info). An
	// empty FontDir uses the engine's embedded defaults. It is caller
	// configuration rather than the card's own data, so it travels on the
	// request instead of in a template
	FontDir string
	// Copyright is the boilerplate line a card's bottom carries. An empty
	// Copyright builds the printed one from the card's own year
	Copyright string
	// DPI is the resolution to render at. It is clamped to the range the
	// template supports, so a preview at a fraction of the authored resolution
	// costs a fraction of the work while laying out identically. Zero renders at
	// the template's own resolution
	DPI int
}

// Report forwards a progress update when a callback is set, so a render's call
// sites stay one line and nil-safe
func (r RenderRequest) Report(step string, frac float64) {
	if r.Progress != nil {
		r.Progress(step, frac)
	}
}

// Template is a template that has been read, checked, and compiled. It never
// changes after it is loaded, so any number of renders can use it at once, and a
// render that has started keeps the template it started with when a library
// replaces it
type Template struct {
	m       *manifest.Manifest
	assets  AssetProvider
	layouts map[string]*Layout
	byShape map[frame.Shape]*Layout
	shapes  []frame.Shape
}

// Load reads a template from its provider, checks the manifest, and compiles it.
// The diagnostics are returned whether or not it loaded, so a caller can show the
// warnings of one that did. An error means the template did not load
func Load(p AssetProvider) (*Template, manifest.Diagnostics, error) {
	raw, err := p.ReadManifest()
	if err != nil {
		return nil, nil, err
	}
	m, ds := manifest.Decode(raw)
	if m != nil {
		ds = append(ds, unsupported(m)...)
	}
	if err := ds.Err(); err != nil {
		return nil, ds, err
	}
	t := &Template{m: m, assets: p, layouts: map[string]*Layout{}, byShape: map[frame.Shape]*Layout{}}
	parts := make(map[string]*Part, len(m.Parts))
	for name, mp := range m.Parts {
		slot, err := frame.ParseSlot(mp.Slot)
		if err != nil {
			return nil, ds, fmt.Errorf("template: part %q: %w", name, err)
		}
		part := &Part{Name: name, Slot: slot, Variants: make(map[string]string, len(mp.Variants)), Origin: image.Pt(mp.Origin.X, mp.Origin.Y), Blend: mp.Blend}
		for key, v := range mp.Variants {
			part.Variants[key] = v.Path
		}
		parts[name] = part
	}
	for name, ml := range m.Layouts {
		l, err := compileLayout(name, ml, parts)
		if err != nil {
			return nil, ds, fmt.Errorf("template: %w", err)
		}
		t.layouts[name] = l
		for _, sh := range l.Serves {
			t.byShape[sh] = l
		}
	}
	for sh := range t.byShape {
		t.shapes = append(t.shapes, sh)
	}
	sort.Slice(t.shapes, func(i, j int) bool {
		if t.shapes[i].Role != t.shapes[j].Role {
			return t.shapes[i].Role < t.shapes[j].Role
		}
		return t.shapes[i].Kind < t.shapes[j].Kind
	})
	return t, ds, nil
}

// unsupported reports what a manifest can say and this engine cannot yet draw, so
// an author is told and a template does not render wrongly without a word
func unsupported(m *manifest.Manifest) manifest.Diagnostics {
	var ds manifest.Diagnostics
	add := func(path, what string) {
		ds = append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: path, Message: what + " is not drawn by this engine yet"})
	}
	for ln, l := range m.Layouts {
		for i, e := range l.Stack {
			if e.Rotate != 0 {
				add(fmt.Sprintf("layouts.%s.stack[%d].rotate", ln, i), "turning an entry")
			}
		}
		for bn, b := range l.TextBoxes {
			if b.Rotate != 0 {
				add(fmt.Sprintf("layouts.%s.textBoxes.%s.rotate", ln, bn), "turning a text box")
			}
		}
		for i, a := range l.Art {
			if a.Rotate != 0 {
				add(fmt.Sprintf("layouts.%s.art[%d].rotate", ln, i), "turning an art window")
			}
		}
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].Path < ds[j].Path })
	return ds
}

// ID is the template's id, such as "mimic.m15"
func (t *Template) ID() string { return t.m.ID }

// Manifest is the template's checked manifest, which must not be changed
func (t *Template) Manifest() *manifest.Manifest { return t.m }

// Assets is where the template's images are read from
func (t *Template) Assets() AssetProvider { return t.assets }

// Close releases the template's provider, when it holds anything that needs
// releasing, such as an open bundle. A render that is still using the template
// must finish first
func (t *Template) Close() error {
	if c, ok := t.assets.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Layout returns the layout that draws a shape, and reports false for one the
// template does not serve
func (t *Template) Layout(sh frame.Shape) (*Layout, bool) {
	l, ok := t.byShape[sh]
	return l, ok
}

// Serves reports whether the template draws a shape
func (t *Template) Serves(sh frame.Shape) bool {
	_, ok := t.byShape[sh]
	return ok
}

// Shapes lists every shape the template draws, sorted
func (t *Template) Shapes() []frame.Shape { return append([]frame.Shape(nil), t.shapes...) }

// Primary is the layout a program asks about the template as a whole, such as how
// big a card renders: the one for an ordinary card when there is one, and otherwise
// the first by name
func (t *Template) Primary() *Layout {
	if l, ok := t.byShape[frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard}]; ok {
		return l
	}
	names := make([]string, 0, len(t.layouts))
	for n := range t.layouts {
		names = append(names, n)
	}
	sort.Strings(names)
	return t.layouts[names[0]]
}

// Feature is a choice a template offers the user
type Feature struct {
	Name string
	manifest.Feature
}

// Features lists what the template lets the user choose, sorted by name
func (t *Template) Features() []Feature {
	out := make([]Feature, 0, len(t.m.Features))
	for name, f := range t.m.Features {
		out = append(out, Feature{Name: name, Feature: f})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Info is what a program shows about a template without drawing with it
type Info struct {
	ID          string
	Name        string
	Description string
	Author      manifest.Author
	License     string
	Credits     []manifest.Credit
	Shapes      []frame.Shape
	Features    []Feature
}

// Info describes the template
func (t *Template) Info() Info {
	return Info{
		ID: t.m.ID, Name: t.m.Name, Description: t.m.Description, Author: t.m.Author,
		License: t.m.License, Credits: t.m.Credits, Shapes: t.Shapes(), Features: t.Features(),
	}
}

// UnsupportedError is what a render returns for a face the template has no layout
// for, so a batch can tell a card no template here renders from one that failed
// partway through
type UnsupportedError struct {
	Template string
	Shape    frame.Shape
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("template %q does not render %s faces", e.Template, e.Shape)
}

// Library holds the templates a program has loaded, by id. It is safe for
// concurrent use. Adding a template whose id is held replaces it, and a render
// that has started keeps the template it holds, so a replaced or removed template
// is closed by whoever loaded it once its renders are done
type Library struct {
	mu   sync.RWMutex
	byID map[string]*Template
}

// NewLibrary returns an empty library
func NewLibrary() *Library { return &Library{byID: map[string]*Template{}} }

// Add puts a template in the library, replacing the one with its id, which it
// returns, or nil when there was none
func (l *Library) Add(t *Template) (replaced *Template) {
	l.mu.Lock()
	defer l.mu.Unlock()
	replaced = l.byID[t.ID()]
	l.byID[t.ID()] = t
	return replaced
}

// Remove takes the template with the id out of the library and returns it, or
// returns nil when there is none
func (l *Library) Remove(id string) *Template {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.byID[id]
	delete(l.byID, id)
	return t
}

// Get returns the template with the id
func (l *Library) Get(id string) (*Template, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	t, ok := l.byID[id]
	return t, ok
}

// List returns every template, sorted by id
func (l *Library) List() []*Template {
	l.mu.RLock()
	out := make([]*Template, 0, len(l.byID))
	for _, t := range l.byID {
		out = append(out, t)
	}
	l.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Serving returns the templates that draw a shape, sorted by id
func (l *Library) Serving(sh frame.Shape) []*Template {
	var out []*Template
	for _, t := range l.List() {
		if t.Serves(sh) {
			out = append(out, t)
		}
	}
	return out
}
