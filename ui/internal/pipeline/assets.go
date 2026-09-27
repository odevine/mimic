package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
	_ "github.com/odevine/mimic/engine/template/all" // registers every template
	"github.com/odevine/mimic/engine/version"
	"github.com/odevine/mimic/ui/internal/catalog"
)

// LooseDirBases are the roots under which a template's loose developer assets
// live as assets/<name>, relative to both a repo-root run and a ui-subdir run.
// Loose assets are developer-local, so a checkout without them falls back to a
// cached bundle or generated placeholders. fontCandidates is the same idea for
// font overrides
var (
	LooseDirBases  = []string{"assets", filepath.Join("..", "assets")}
	fontCandidates = []string{"local-fonts", filepath.Join("..", "local-fonts")}
)

// LocalVersion is the synthetic version label for a template rendered from its
// loose developer directory rather than a downloaded bundle
const LocalVersion = "local"

// Source records which tier of the fallback chain a template resolved from,
// so the caller knows whether to attempt a background bundle update. A loose
// developer directory wins by design and suppresses the download
type Source int

const (
	SourceLoose Source = iota
	SourceBundle
	SourcePlaceholder
	// SourceExplicit is a selection restored from a saved preference. Like a
	// loose dir it is the user's choice, so the background auto-update leaves it
	SourceExplicit
)

// Resolve builds the startup template from a network-free fallback
// chain: a loose developer directory, then the newest cached bundle, then
// generated placeholders. It never blocks on the network, so a fresh offline
// Install still renders. The returned source lets the caller decide whether to
// look for an update
func Resolve(name string) (*Template, Source, error) {
	tmpl, err := template.Get(name)
	if err != nil {
		return nil, SourcePlaceholder, err
	}
	if dir := LooseDir(name); dir != "" {
		return &Template{
			Name: name, Version: LocalVersion, template: tmpl, fontDir: resolveFontDir(),
			provider: template.NewFSAssetProvider(dir), cleanup: func() {},
		}, SourceLoose, nil
	}
	if ver, ok := catalog.NewestCachedVersion(name); ok {
		if at, err := activeFromCachedBundle(name, ver, tmpl); err == nil {
			return at, SourceBundle, nil
		} else {
			log.Printf("mimic-ui: skipping cached bundle %s %s: %v", name, ver, err)
		}
	}
	at, err := placeholderActive(name, tmpl)
	if err != nil {
		return nil, SourcePlaceholder, err
	}
	return at, SourcePlaceholder, nil
}

// FromVersion builds the template for an explicit selection. The synthetic
// local version uses the loose directory; any other version is ensured in the
// cache (downloaded and verified when missing) and opened as a bundle. progress
// may be nil
func FromVersion(ctx context.Context, name, version string, progress func(done, total int64)) (*Template, error) {
	tmpl, err := template.Get(name)
	if err != nil {
		return nil, err
	}
	if version == LocalVersion {
		dir := LooseDir(name)
		if dir == "" {
			return nil, fmt.Errorf("no local assets for %q", name)
		}
		return &Template{
			Name: name, Version: LocalVersion, template: tmpl, fontDir: resolveFontDir(),
			provider: template.NewFSAssetProvider(dir), cleanup: func() {},
		}, nil
	}
	if _, err := catalog.EnsureVersion(ctx, name, version, progress); err != nil {
		return nil, err
	}
	return activeFromCachedBundle(name, version, tmpl)
}

// Latest builds the template for the newest catalog version the engine
// can render, downloading it when needed. It is the background-update path, so a
// missing catalog or no compatible version is a returned error the caller logs
func Latest(ctx context.Context, name string) (*Template, error) {
	idx, err := catalog.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	t, ok := idx.Find(name)
	if !ok {
		return nil, fmt.Errorf("template %q not in catalog", name)
	}
	v, ok := catalog.PickCompatible(t)
	if !ok {
		return nil, fmt.Errorf("no version of %q is compatible with engine %s", name, version.Version)
	}
	return FromVersion(ctx, name, v.Version, nil)
}

// activeFromCachedBundle opens a cached bundle and pairs it with a template
func activeFromCachedBundle(name, version string, tmpl template.Template) (*Template, error) {
	path, err := catalog.BundlePath(name, version)
	if err != nil {
		return nil, err
	}
	p, err := template.NewZipAssetProvider(path)
	if err != nil {
		return nil, err
	}
	return &Template{
		Name: name, Version: version, template: tmpl, fontDir: resolveFontDir(),
		provider: p, cleanup: func() { p.Close() },
	}, nil
}

// WritePlaceholders writes a template's stand-in assets into a directory. Tests
// swap in a copy of assets generated once, since encoding them is slow
var WritePlaceholders = render.WritePlaceholderAssets

// placeholderActive generates stand-in assets for a template that has no loose
// directory and no cached bundle
func placeholderActive(name string, tmpl template.Template) (*Template, error) {
	tmp, err := os.MkdirTemp("", "mimic-placeholder-")
	if err != nil {
		return nil, err
	}
	if err := WritePlaceholders(tmp, name); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	return &Template{
		Name: name, Version: "", template: tmpl, fontDir: resolveFontDir(),
		provider: template.NewFSAssetProvider(tmp), cleanup: func() { os.RemoveAll(tmp) },
	}, nil
}

// LooseDir returns the loose developer asset directory for a template, or "" if
// none exists
func LooseDir(name string) string {
	for _, base := range LooseDirBases {
		cand := filepath.Join(base, name)
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			return cand
		}
	}
	return ""
}

// resolveFontDir returns the first font-override directory that exists, or ""
// to use the engine's embedded default fonts
func resolveFontDir() string {
	for _, cand := range fontCandidates {
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			return cand
		}
	}
	return ""
}

// Display names a template and version for the indicator. An empty version is
// the placeholder fallback
func Display(name, version string) string {
	switch version {
	case "":
		return name + " (placeholder)"
	case LocalVersion:
		return name + " · local"
	default:
		return name + " · " + version
	}
}
