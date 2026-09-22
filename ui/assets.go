package main

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
)

// looseDirBases are the roots under which a template's loose developer assets
// live as assets/<name>, relative to both a repo-root run and a ui-subdir run.
// Loose assets are developer-local, so a checkout without them falls back to a
// cached bundle or generated placeholders. fontCandidates is the same idea for
// font overrides
var (
	looseDirBases  = []string{"assets", filepath.Join("..", "assets")}
	fontCandidates = []string{"local-fonts", filepath.Join("..", "local-fonts")}
)

// localVersion is the synthetic version label for a template rendered from its
// loose developer directory rather than a downloaded bundle
const localVersion = "local"

// assetSource records which tier of the fallback chain a template resolved from,
// so the caller knows whether to attempt a background bundle update. A loose
// developer directory wins by design and suppresses the download
type assetSource int

const (
	sourceLoose assetSource = iota
	sourceBundle
	sourcePlaceholder
	// sourceExplicit is a selection restored from a saved preference. Like a
	// loose dir it is the user's choice, so the background auto-update leaves it
	sourceExplicit
)

// resolveActiveTemplate builds the startup template from a network-free fallback
// chain: a loose developer directory, then the newest cached bundle, then
// generated placeholders. It never blocks on the network, so a fresh offline
// install still renders. The returned source lets the caller decide whether to
// look for an update
func resolveActiveTemplate(name string) (*activeTemplate, assetSource, error) {
	tmpl, err := template.Get(name)
	if err != nil {
		return nil, sourcePlaceholder, err
	}
	if dir := looseDir(name); dir != "" {
		return &activeTemplate{
			name: name, version: localVersion, template: tmpl, fontDir: resolveFontDir(),
			provider: template.NewFSAssetProvider(dir), cleanup: func() {},
		}, sourceLoose, nil
	}
	if ver, ok := newestCachedVersion(name); ok {
		if at, err := activeFromCachedBundle(name, ver, tmpl); err == nil {
			return at, sourceBundle, nil
		} else {
			log.Printf("mimic-ui: skipping cached bundle %s %s: %v", name, ver, err)
		}
	}
	at, err := placeholderActive(name, tmpl)
	if err != nil {
		return nil, sourcePlaceholder, err
	}
	return at, sourcePlaceholder, nil
}

// activeFromVersion builds the template for an explicit selection. The synthetic
// local version uses the loose directory; any other version is ensured in the
// cache (downloaded and verified when missing) and opened as a bundle. progress
// may be nil
func activeFromVersion(ctx context.Context, name, version string, progress func(done, total int64)) (*activeTemplate, error) {
	tmpl, err := template.Get(name)
	if err != nil {
		return nil, err
	}
	if version == localVersion {
		dir := looseDir(name)
		if dir == "" {
			return nil, fmt.Errorf("no local assets for %q", name)
		}
		return &activeTemplate{
			name: name, version: localVersion, template: tmpl, fontDir: resolveFontDir(),
			provider: template.NewFSAssetProvider(dir), cleanup: func() {},
		}, nil
	}
	if _, err := ensureVersion(ctx, name, version, progress); err != nil {
		return nil, err
	}
	return activeFromCachedBundle(name, version, tmpl)
}

// autoLatestActive builds the template for the newest catalog version the engine
// can render, downloading it when needed. It is the background-update path, so a
// missing catalog or no compatible version is a returned error the caller logs
func autoLatestActive(ctx context.Context, name string) (*activeTemplate, error) {
	idx, err := fetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	t, ok := idx.findTemplate(name)
	if !ok {
		return nil, fmt.Errorf("template %q not in catalog", name)
	}
	v, ok := pickCompatible(t)
	if !ok {
		return nil, fmt.Errorf("no version of %q is compatible with engine %s", name, version.Version)
	}
	return activeFromVersion(ctx, name, v.Version, nil)
}

// activeFromCachedBundle opens a cached bundle and pairs it with a template
func activeFromCachedBundle(name, version string, tmpl template.Template) (*activeTemplate, error) {
	path, err := bundlePath(name, version)
	if err != nil {
		return nil, err
	}
	p, err := template.NewZipAssetProvider(path)
	if err != nil {
		return nil, err
	}
	return &activeTemplate{
		name: name, version: version, template: tmpl, fontDir: resolveFontDir(),
		provider: p, cleanup: func() { p.Close() },
	}, nil
}

// placeholderActive generates stand-in assets for a template that has no loose
// directory and no cached bundle
func placeholderActive(name string, tmpl template.Template) (*activeTemplate, error) {
	tmp, err := os.MkdirTemp("", "mimic-placeholder-")
	if err != nil {
		return nil, err
	}
	if err := render.WritePlaceholderAssets(tmp, name); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	return &activeTemplate{
		name: name, version: "", template: tmpl, fontDir: resolveFontDir(),
		provider: template.NewFSAssetProvider(tmp), cleanup: func() { os.RemoveAll(tmp) },
	}, nil
}

// looseDir returns the loose developer asset directory for a template, or "" if
// none exists
func looseDir(name string) string {
	for _, base := range looseDirBases {
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
