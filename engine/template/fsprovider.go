package template

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// maxDimension caps a manifest's width or height. A manifest's numbers become
// buffer dimensions downstream, so they are validated before anything sized
// from them is allocated
const maxDimension = 20000

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

// Manifest reads and validates the manifest
func (p *FSAssetProvider) Manifest() (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(p.root, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("template: reading manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("template: decoding manifest: %w", err)
	}
	if err := validateManifest(&m); err != nil {
		return nil, err
	}
	return &m, nil
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

func validateManifest(m *Manifest) error {
	if m.Width <= 0 || m.Height <= 0 {
		return fmt.Errorf("template: manifest has non-positive dimensions %dx%d", m.Width, m.Height)
	}
	if m.Width > maxDimension || m.Height > maxDimension {
		return fmt.Errorf("template: manifest dimensions %dx%d exceed %d limit", m.Width, m.Height, maxDimension)
	}
	return nil
}
