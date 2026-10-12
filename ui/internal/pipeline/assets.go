package pipeline

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/engine/version"
	"github.com/odevine/mimic/ui/internal/catalog"
)

// layerCaches is the budget each template's layer cache gets and the caches
// currently holding layers, so a change of size reaches templates already loaded
var layerCaches = struct {
	sync.Mutex
	budget int64
	live   map[*template.CachedAssets]struct{}
}{budget: template.DefaultImageCacheBytes, live: map[*template.CachedAssets]struct{}{}}

// SetLayerCacheBytes sets how many bytes of decoded frame layers each template
// keeps in memory to share between cards. It resizes the caches of templates
// already loaded, and applies to the ones loaded later
func SetLayerCacheBytes(n int64) {
	layerCaches.Lock()
	defer layerCaches.Unlock()
	layerCaches.budget = n
	for c := range layerCaches.live {
		c.SetBudget(n)
	}
}

// cached wraps a provider so the frame layers a batch shares are decoded once.
// Releasing the template releases the cache, and the provider with it when it
// can be closed. after runs last, for whatever the provider's files need
func cached(p template.AssetProvider, after func()) (template.AssetProvider, func()) {
	layerCaches.Lock()
	c := template.NewCachedAssets(p, layerCaches.budget)
	layerCaches.live[c] = struct{}{}
	layerCaches.Unlock()
	return c, func() {
		forget(c)
		c.Close()
		if after != nil {
			after()
		}
	}
}

// forget takes a cache out of the set that resizes with the setting
func forget(c *template.CachedAssets) {
	layerCaches.Lock()
	delete(layerCaches.live, c)
	layerCaches.Unlock()
}

// dropLayers empties a template's layer cache for good, for one that was swapped
// out. A render still reading from it decodes as it goes, and the template's
// files stay open until shutdown like before
func (t *Template) dropLayers() {
	if c, ok := t.provider.(*template.CachedAssets); ok {
		forget(c)
		c.SetBudget(0)
	}
}

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

// Resolve builds the startup template from a network-free fallback chain: the
// loose developer folder of the template with the id, then its newest cached
// bundle, then the placeholder template. It never blocks on the network, so a
// fresh offline install still renders. The returned source lets the caller decide
// whether to look for an update. The placeholder has an id of its own
func Resolve(id string) (*Template, Source, error) {
	if dir := LooseDir(id); dir != "" {
		at, err := openLoose(id, dir)
		if err != nil {
			return nil, SourceLoose, err
		}
		return at, SourceLoose, nil
	}
	if ver, ok := catalog.NewestCachedVersion(id); ok {
		if at, err := activeFromCachedBundle(id, ver); err == nil {
			return at, SourceBundle, nil
		} else {
			log.Printf("mimic-ui: skipping cached bundle %s %s: %v", id, ver, err)
		}
	}
	at, err := placeholderActive()
	if err != nil {
		return nil, SourcePlaceholder, err
	}
	return at, SourcePlaceholder, nil
}

// FromVersion builds the template for an explicit selection. The synthetic
// local version uses the loose directory; any other version is ensured in the
// cache (downloaded and verified when missing) and opened as a bundle. progress
// may be nil
func FromVersion(ctx context.Context, id, version string, progress func(done, total int64)) (*Template, error) {
	if version == LocalVersion {
		dir := LooseDir(id)
		if dir == "" {
			return nil, fmt.Errorf("no local assets for %q", id)
		}
		return openLoose(id, dir)
	}
	if _, err := catalog.EnsureVersion(ctx, id, version, progress); err != nil {
		return nil, err
	}
	return activeFromCachedBundle(id, version)
}

// Latest builds the template for the newest catalog version the engine
// can render, downloading it when needed. It is the background-update path, so a
// missing catalog or no compatible version is a returned error the caller logs
func Latest(ctx context.Context, id string) (*Template, error) {
	idx, err := catalog.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	t, ok := idx.Find(id)
	if !ok {
		return nil, fmt.Errorf("template %q not in catalog", id)
	}
	v, ok := catalog.PickCompatible(t)
	if !ok {
		return nil, fmt.Errorf("no version of %q is compatible with engine %s", id, version.Version)
	}
	return FromVersion(ctx, id, v.Version, nil)
}

// activeFromCachedBundle opens a cached bundle
func activeFromCachedBundle(id, version string) (*Template, error) {
	path, err := catalog.BundlePath(id, version)
	if err != nil {
		return nil, err
	}
	zp, err := template.NewZipAssetProvider(path)
	if err != nil {
		return nil, err
	}
	at, err := open(id, version, zp, nil)
	if err != nil {
		zp.Close()
		return nil, err
	}
	return at, nil
}

// openLoose loads the template in a loose developer folder, and notes what its
// files look like so a later change is noticed
func openLoose(id, dir string) (*Template, error) {
	stamp := looseStamp(dir)
	at, err := open(id, LocalVersion, template.NewFSAssetProvider(dir), nil)
	if err != nil {
		return nil, err
	}
	at.dir, at.stamp = dir, stamp
	return at, nil
}

// looseStamp is the names, sizes, and modification times of a loose folder's
// manifest and of every image the manifest names, which changes when an author
// edits any of them. A folder that cannot be read has an empty stamp
func looseStamp(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return ""
	}
	m, _ := manifest.Decode(raw)
	var b strings.Builder
	add := func(rel string) {
		if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err == nil {
			fmt.Fprintf(&b, "%s|%d|%d;", rel, st.Size(), st.ModTime().UnixNano())
		}
	}
	add("manifest.json")
	if m != nil {
		for _, name := range slices.Sorted(maps.Keys(m.Parts)) {
			for _, key := range slices.Sorted(maps.Keys(m.Parts[name].Variants)) {
				add(m.Parts[name].Variants[key].Path)
			}
		}
	}
	return b.String()
}

// reloadChanged loads again any loose folder template that is in use and whose
// files have changed since it was loaded, so an author sees an edit in the next
// render. It looks at most once a second. A template that no longer loads is kept
// as it was, and the error is logged
func (p *Pipeline) reloadChanged() {
	p.mu.Lock()
	if time.Since(p.checked) < time.Second {
		p.mu.Unlock()
		return
	}
	p.checked = time.Now()
	active := p.active.Load()
	loaded := maps.Clone(p.loaded)
	p.mu.Unlock()

	stale := func(at *Template) bool { return at != nil && at.dir != "" && looseStamp(at.dir) != at.stamp }
	if stale(active) {
		if fresh, err := openLoose(active.Name, active.dir); err == nil {
			p.Install(fresh)
		} else {
			log.Printf("mimic-ui: reloading %s: %v", active.Name, err)
		}
	}
	for id, at := range loaded {
		if !stale(at) {
			continue
		}
		fresh, err := openLoose(id, at.dir)
		if err != nil {
			log.Printf("mimic-ui: reloading %s: %v", id, err)
			continue
		}
		p.mu.Lock()
		if p.loaded[id] == at {
			p.loaded[id] = fresh
			p.cleanups = append(p.cleanups, fresh.cleanup)
			at.dropLayers()
		} else {
			fresh.Close()
		}
		p.mu.Unlock()
	}
}

// placeholderActive is the template for an app with nothing installed. Its
// version is empty
func placeholderActive() (*Template, error) {
	return open(render.PlaceholderID, "", render.PlaceholderAssets(), nil)
}

// open loads a template from a provider behind a layer cache, and checks that it
// is the template that was asked for. Releasing the template releases the cache
// and the provider with it
func open(id, version string, p template.AssetProvider, after func()) (*Template, error) {
	cp, cleanup := cached(p, after)
	tmpl, ds, err := template.Load(cp)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("loading %s: %w", id, err)
	}
	if tmpl.ID() != id {
		cleanup()
		return nil, fmt.Errorf("expected template %s, and it is %s", id, tmpl.ID())
	}
	for _, d := range ds {
		if d.Severity == manifest.Warning {
			log.Printf("mimic-ui: template %s: %v", id, d)
		}
	}
	return &Template{Name: id, Version: version, tmpl: tmpl, provider: cp, cleanup: cleanup}, nil
}

// Display names a template and version for the indicator, by the template's title.
// An empty version is the placeholder fallback, which is no template at all
func Display(title, version string) string {
	switch version {
	case "":
		return NoTemplateLabel
	case LocalVersion:
		return title + " · local"
	default:
		return title + " · " + version
	}
}

// NoTemplateLabel is what the indicator says when no template is installed
const NoTemplateLabel = "No template installed"
