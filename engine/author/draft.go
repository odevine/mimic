package author

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

// Draft renders a template straight from its source folder. Build cuts every
// image and writes a bundle, which is the right cost for a release and too slow
// for an editor that renders after each change. A Draft reads the images where the
// author keeps them and shares one cache of decoded images between the templates it
// opens, so a change to a layout costs a resolve and a render and not a pack
//
// A Draft is safe for concurrent use. A template it has opened keeps working after
// a later Open, because the images it reads are named by the same paths
type Draft struct {
	files *draftFiles
	cache *template.CachedAssets
	mu    sync.Mutex
}

// NewDraft serves the source folder in fsys. budget is the most decoded image data
// the draft keeps, as for template.NewCachedAssets
func NewDraft(fsys fs.FS, budget int64) *Draft {
	files := &draftFiles{fsys: fsys}
	return &Draft{files: files, cache: template.NewCachedAssets(files, budget)}
}

// Open reads the folder as it is now and returns the template it describes. The
// diagnostics are every problem found, with the same checks Validate makes, and the
// template is nil when any is an error
func (d *Draft) Open(opt Options) (*template.Template, manifest.Diagnostics, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ds := resolveSource(d.files.fsys, opt)
	if r == nil {
		return nil, ds, ds.Err()
	}
	// Fonts are read through the provider by the path the manifest names, so the
	// hash and names the check filled in are written into the manifest it loads
	raw, err := json.Marshal(r.m)
	if err != nil {
		return nil, ds, err
	}
	d.files.setManifest(raw)
	t, lds, err := template.Load(d.cache)
	ds = append(ds, lds...)
	if err != nil {
		return nil, ds, fmt.Errorf("author: %w", err)
	}
	return t, ds, nil
}

// Forget drops the decoded copies of the image at path, a slash separated path from
// the template's folder, so a file that has changed is read again. The editor calls
// it after replacing an image. Paths that did not change keep their decoded images
func (d *Draft) Forget(path string) { d.cache.Forget(path) }

// Close drops every decoded image
func (d *Draft) Close() error { return d.cache.Close() }

// draftFiles is the provider under a Draft's cache. It serves the images in the
// folder by their source paths, and the manifest of the latest Open
type draftFiles struct {
	fsys     fs.FS
	mu       sync.Mutex
	manifest []byte
}

func (f *draftFiles) setManifest(raw []byte) {
	f.mu.Lock()
	f.manifest = raw
	f.mu.Unlock()
}

// ReadManifest returns the manifest of the latest Open
func (f *draftFiles) ReadManifest() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.manifest == nil {
		return nil, fmt.Errorf("author: the draft has not been opened")
	}
	return f.manifest, nil
}

// Open returns a file of the folder, and fails for a path that is not inside it
func (f *draftFiles) Open(rel string) (io.ReadCloser, error) {
	if !fs.ValidPath(rel) {
		return nil, fmt.Errorf("author: path %q is not inside the template", rel)
	}
	return f.fsys.Open(rel)
}
