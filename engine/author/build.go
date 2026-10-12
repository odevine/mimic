package author

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"io/fs"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/engine/version"
)

// Options changes what a build does
type Options struct {
	// Version replaces the version in template.json, for a build that adds a
	// suffix such as a pre-release
	Version string
}

// Built is a template in its resolved form with the images it refers to
type Built struct {
	Header   template.BundleHeader
	Manifest *manifest.Manifest
	// ManifestJSON is the bytes of the manifest as a bundle holds it
	ManifestJSON []byte
	// Assets holds each image by its path in the bundle
	Assets map[string][]byte
}

var semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// Build reads the source form in fsys and resolves it. Every problem found is in
// the diagnostics, and the result is nil when there is an error
func Build(fsys fs.FS, opt Options) (*Built, manifest.Diagnostics) {
	return buildFrom(fsys, opt, true)
}

// Validate checks a template's source folder the way Build does, without cutting or
// storing an image, and is strict where the engine's loader is lenient: a field the
// contract does not have is an error. It reports every problem in one pass
func Validate(fsys fs.FS, opt Options) manifest.Diagnostics {
	_, ds := buildFrom(fsys, opt, false)
	return ds
}

// buildFrom is Build, and with write unset it reads each image to check it and leaves
// the result without assets
func buildFrom(fsys fs.FS, opt Options, write bool) (*Built, manifest.Diagnostics) {
	src, ds := readSource(fsys)
	if src == nil {
		return nil, ds
	}
	if opt.Version != "" {
		src.Version = opt.Version
	}
	ds = append(ds, checkHeader(src)...)

	files, lds := readLayouts(fsys)
	ds = append(ds, lds...)
	var layouts map[string]rawLayout
	if !lds.HasErrors() {
		var rds manifest.Diagnostics
		layouts, rds = resolveLayouts(files)
		ds = append(ds, rds...)
	}

	// The parts are checked and cut as the layouts are resolved, so one pass shows an
	// author every problem
	sources := map[string]map[string]string{}
	parts := map[string]manifest.Part{}
	unread := map[string]bool{}
	for _, name := range sortedKeys(src.Parts) {
		p := src.Parts[name]
		vs, pds := expandPart(fsys, name, p)
		ds = append(ds, pds...)
		unread[name] = pds.HasErrors()
		sources[name] = vs
		mp := manifest.Part{Slot: p.Slot, Origin: p.Origin, Blend: p.Blend, Variants: map[string]manifest.Variant{}}
		for key, path := range vs {
			mp.Variants[key] = manifest.Variant{Path: path}
		}
		parts[name] = mp
	}
	if layouts == nil {
		sortDiagnostics(ds)
		return nil, ds
	}

	// Decoding the manifest as written, before any image is cut, checks it against
	// the engine's own rules with the source paths in it
	full := rawManifest{
		Contract: src.Contract, ID: src.ID, Name: src.Name, Description: src.Description,
		Author: src.Author, License: src.License, Credits: src.Credits,
		Features: src.Features, Parts: parts, Layouts: layouts,
	}
	raw, err := json.Marshal(full)
	if err != nil {
		return nil, append(ds, manifest.Diagnostic{Severity: manifest.Error, Message: err.Error()})
	}
	m, mds := manifest.DecodeStrict(raw)
	for _, d := range mds {
		// A part whose folder could not be read has been reported once already
		if name, ok := strings.CutPrefix(d.Path, "parts."); ok && unread[strings.Split(name, ".")[0]] && strings.HasSuffix(d.Path, ".variants") {
			continue
		}
		ds = append(ds, d)
	}
	if m == nil || ds.HasErrors() {
		sortDiagnostics(ds)
		return nil, ds
	}

	store := &assetStore{check: !write}
	cuts := packParts(fsys, sources, m, store, &ds)
	if ds.HasErrors() || !write {
		sortDiagnostics(ds)
		return nil, ds
	}
	for lname, l := range m.Layouts {
		for i, e := range l.Stack {
			if e.At != nil {
				off := cuts[e.Part]
				l.Stack[i].At = &manifest.Point{X: e.At.X + off.X, Y: e.At.Y + off.Y}
			}
		}
		m.Layouts[lname] = l
	}

	out, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return nil, append(ds, manifest.Diagnostic{Severity: manifest.Error, Message: err.Error()})
	}
	if len(out) > manifest.MaxBytes {
		ds = append(ds, manifest.Diagnostic{Severity: manifest.Error, Message: fmt.Sprintf("the manifest is %d bytes, over the limit of %d", len(out), manifest.MaxBytes)})
		return nil, ds
	}
	// The manifest as bundled must pass the same rules the engine applies on load
	if _, fds := manifest.DecodeStrict(out); fds.HasErrors() {
		sortDiagnostics(fds)
		return nil, append(ds, fds...)
	}
	sortDiagnostics(ds)
	return &Built{
		Header:       template.BundleHeader{Format: template.BundleFormat, ID: src.ID, Version: src.Version, MinEngine: src.MinEngine},
		Manifest:     m,
		ManifestJSON: out,
		Assets:       store.files,
	}, ds
}

// rawManifest is the manifest with its layouts as they were resolved
type rawManifest struct {
	Contract    string                      `json:"contract"`
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	Description string                      `json:"description,omitempty"`
	Author      manifest.Author             `json:"author,omitempty"`
	License     string                      `json:"license,omitempty"`
	Credits     []manifest.Credit           `json:"credits,omitempty"`
	Parts       map[string]manifest.Part    `json:"parts"`
	Layouts     map[string]rawLayout        `json:"layouts"`
	Features    map[string]manifest.Feature `json:"features,omitempty"`
}

// packParts cuts and stores the images of every part in the manifest, a few at a
// time, and returns the offset each part was cut by
func packParts(fsys fs.FS, sources map[string]map[string]string, m *manifest.Manifest, store *assetStore, ds *manifest.Diagnostics) map[string]image.Point {
	names := sortedKeys(m.Parts)
	held := make(map[string]manifest.Part, len(m.Parts))
	for k, v := range m.Parts {
		held[k] = v
	}
	cuts := make(map[string]image.Point, len(names))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, runtime.GOMAXPROCS(0)/2))
	for _, name := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			p, off, pds := packPart(fsys, name, sources[name], held[name], store)
			mu.Lock()
			defer mu.Unlock()
			*ds = append(*ds, pds...)
			if !pds.HasErrors() {
				m.Parts[name] = p
				cuts[name] = off
			}
		}()
	}
	wg.Wait()
	return cuts
}

func checkHeader(s *Source) manifest.Diagnostics {
	var ds manifest.Diagnostics
	bad := func(path, format string, args ...any) {
		ds = append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: sourceFile + ": " + path, Message: fmt.Sprintf(format, args...)})
	}
	if !semverRe.MatchString(s.Version) {
		bad("version", "%q is not a version such as 1.2.3", s.Version)
	}
	if s.MinEngine != "" {
		switch {
		case !semverRe.MatchString(s.MinEngine):
			bad("minEngine", "%q is not a version such as 1.2.3", s.MinEngine)
		case !version.Satisfies(s.MinEngine):
			bad("minEngine", "the template needs engine %s and this is %s", s.MinEngine, version.Version)
		}
	}
	return ds
}

// WriteZip writes the bundle as a zip. The same Built is the same bytes every time,
// since the entries are in a fixed order with a fixed date
func (b *Built) WriteZip(w io.Writer) error {
	zw := zip.NewWriter(w)
	header, err := json.MarshalIndent(b.Header, "", " ")
	if err != nil {
		return err
	}
	if err := writeEntry(zw, "bundle.json", zip.Deflate, header); err != nil {
		return err
	}
	if err := writeEntry(zw, "manifest.json", zip.Deflate, b.ManifestJSON); err != nil {
		return err
	}
	names := make([]string, 0, len(b.Assets))
	for n := range b.Assets {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		// A PNG is compressed already
		if err := writeEntry(zw, n, zip.Store, b.Assets[n]); err != nil {
			return err
		}
	}
	return zw.Close()
}

// zipDate is 1980-01-01, the earliest date a zip can hold
const zipDate = 1<<5 | 1

func writeEntry(zw *zip.Writer, name string, method uint16, data []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method, ModifiedDate: zipDate})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
