package template

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/odevine/mimic/engine/manifest"
)

// FSAssetProvider reads manifest.json and layer PNGs from a directory on disk.
// It is the only AssetProvider today. The interface exists so other backings
// can stand in without any template code changing
type FSAssetProvider struct {
	root string
}

// NewFSAssetProvider roots a provider at dir. The manifest is expected at
// dir/manifest.json and layer paths resolve relative to dir
func NewFSAssetProvider(dir string) *FSAssetProvider {
	return &FSAssetProvider{root: dir}
}

// ReadManifest reads manifest.json, which is no larger than a manifest may be
func (p *FSAssetProvider) ReadManifest() ([]byte, error) {
	f, err := os.Open(filepath.Join(p.root, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("template: reading manifest: %w", err)
	}
	defer f.Close()
	return readManifest(f)
}

// Open returns a reader for a layer PNG at relPath, joined against the root and
// guarded so it cannot resolve outside it. The guard matters when a manifest is
// sourced from somewhere less trusted than the shipped template
func (p *FSAssetProvider) Open(relPath string) (io.ReadCloser, error) {
	full, err := p.resolve(relPath)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

// resolve joins relPath against the root and verifies the result stays inside
// it, defeating "../" traversal
func (p *FSAssetProvider) resolve(relPath string) (string, error) {
	root, err := filepath.Abs(p.root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", fmt.Errorf("template: resolving %q: %w", relPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("template: path %q escapes asset root", relPath)
	}
	return full, nil
}

// safeRelPath cleans a slash-separated asset name and rejects any that is
// absolute or escapes its root with "..". Both providers guard the same way, so
// a manifest from a less trusted source cannot reach outside its own tree
// whether the tree is a directory or a zip. It returns the cleaned name for
// lookup
func safeRelPath(relPath string) (string, error) {
	clean := path.Clean(filepath.ToSlash(relPath))
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("template: path %q escapes asset root", relPath)
	}
	return clean, nil
}

// readManifest reads a manifest from r, and fails for one over the size a manifest
// may be, so a file or a bundle entry that claims to be one cannot make the engine
// read without end
func readManifest(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, manifest.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("template: reading manifest: %w", err)
	}
	if len(raw) > manifest.MaxBytes {
		return nil, fmt.Errorf("template: the manifest is over the %d byte limit", manifest.MaxBytes)
	}
	return raw, nil
}
