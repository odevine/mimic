package pipeline

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/catalog"
)

// DefaultTemplate is the id of the template the app starts with
const DefaultTemplate = "mimic.m15"

// LooseDirBases are the roots under which a template's loose developer assets
// live as assets/<folder>, relative to both a repo-root run and a ui-subdir run.
// Loose assets are developer-local, so a checkout without them falls back to a
// cached bundle or the placeholder
var LooseDirBases = []string{"assets", filepath.Join("..", "assets")}

// idRe is the form of a template id, which is also the name of its folder in the
// bundle cache. A folder in the cache with another name is left over from an
// earlier format and is not a template
var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.[a-z0-9][a-z0-9-]*$`)

// ValidID reports whether a name has the form of a template id, which is the name
// of its folder in the bundle cache
func ValidID(id string) bool { return idRe.MatchString(id) }

// Installed is a template that can render without a download: it has a loose
// developer folder or a cached bundle
type Installed struct {
	ID string
	template.Info
	// Template is the template as read from its manifest, with no images behind
	// it, which is enough to resolve feature values against what it declares
	Template *template.Template
}

// peeked is what reading a template's manifest told, kept until the file changes
type peeked struct {
	tmpl *template.Template
	err  error
}

var peekCache = struct {
	sync.Mutex
	byStamp map[string]peeked
}{byStamp: map[string]peeked{}}

// peek reads a template's manifest to learn its id, shapes, and features without
// loading any image. The result is kept by the file's size and modification time
func peek(stampPath string, open func() (template.AssetProvider, func(), error)) (*template.Template, error) {
	st, err := os.Stat(stampPath)
	if err != nil {
		return nil, err
	}
	stamp := fmt.Sprintf("%s|%s|%d", stampPath, st.ModTime(), st.Size())
	peekCache.Lock()
	if p, ok := peekCache.byStamp[stamp]; ok {
		peekCache.Unlock()
		return p.tmpl, p.err
	}
	peekCache.Unlock()

	p, closeFn, err := open()
	var t *template.Template
	if err == nil {
		t, _, err = template.Load(p)
		closeFn()
	}
	peekCache.Lock()
	peekCache.byStamp[stamp] = peeked{t, err}
	peekCache.Unlock()
	if err != nil {
		// Said once, since the result is kept until the file changes
		log.Printf("mimic-ui: skipping %s: %v", stampPath, err)
	}
	return t, err
}

func peekDir(dir string) (*template.Template, error) {
	return peek(filepath.Join(dir, "manifest.json"), func() (template.AssetProvider, func(), error) {
		return template.NewFSAssetProvider(dir), func() {}, nil
	})
}

func peekBundle(path string) (*template.Template, error) {
	return peek(path, func() (template.AssetProvider, func(), error) {
		zp, err := template.NewZipAssetProvider(path)
		if err != nil {
			return nil, nil, err
		}
		return zp, func() { zp.Close() }, nil
	})
}

// looseTemplates lists the loose developer folders that hold a template, by the
// id in their manifest. A root earlier in LooseDirBases wins a repeated id
func looseTemplates() map[string]string {
	out := map[string]string{}
	for _, base := range LooseDirBases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			dir := filepath.Join(base, e.Name())
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
				continue
			}
			t, err := peekDir(dir)
			if err != nil {
				continue
			}
			if _, held := out[t.ID()]; !held {
				out[t.ID()] = dir
			}
		}
	}
	return out
}

// LooseDir returns the loose developer folder that holds the template with the
// id, or "" if none does
func LooseDir(id string) string { return looseTemplates()[id] }

// cachedIDs lists the templates that have a bundle in the cache
func cachedIDs() []string {
	dir, err := catalog.Dir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && idRe.MatchString(e.Name()) {
			if _, ok := catalog.NewestCachedVersion(e.Name()); ok {
				out = append(out, e.Name())
			}
		}
	}
	return out
}

// InstalledTemplates lists the templates that can render without a download,
// sorted by id. A template with a loose folder is described by it, and otherwise
// by its newest cached bundle. One whose files cannot be read is left out
func InstalledTemplates() []Installed {
	loose := looseTemplates()
	ids := map[string]bool{}
	for id := range loose {
		ids[id] = true
	}
	for _, id := range cachedIDs() {
		ids[id] = true
	}
	var out []Installed
	for id := range ids {
		var t *template.Template
		var err error
		if dir, ok := loose[id]; ok {
			t, err = peekDir(dir)
		} else {
			ver, _ := catalog.NewestCachedVersion(id)
			var path string
			if path, err = catalog.BundlePath(id, ver); err == nil {
				t, err = peekBundle(path)
			}
		}
		if err != nil {
			log.Printf("mimic-ui: skipping template %s: %v", id, err)
			continue
		}
		out = append(out, Installed{ID: id, Info: t.Info(), Template: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// InstalledInfo describes an installed template
func InstalledInfo(id string) (Installed, bool) {
	for _, in := range InstalledTemplates() {
		if in.ID == id {
			return in, true
		}
	}
	return Installed{}, false
}

// installed reports whether a template can render without a download. The
// placeholder does not count, since it would render a face as flat boxes
func installed(id string) bool {
	_, ok := InstalledInfo(id)
	return ok
}

// InstalledVersions lists the versions of a template that render without a
// download: the local developer version first, then cached bundles, newest
// first
func InstalledVersions(id string) []string {
	var out []string
	if LooseDir(id) != "" {
		out = append(out, LocalVersion)
	}
	cached, _ := catalog.CachedVersions(id)
	return append(out, cached...)
}
