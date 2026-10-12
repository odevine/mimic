package template

import (
	"bytes"
	"fmt"
	"io"
	"path"
)

// MemoryAssets is an asset provider that holds a manifest and its images in memory,
// for a template a program builds itself instead of reading from disk
type MemoryAssets struct {
	manifest []byte
	files    map[string][]byte
}

// NewMemoryAssets serves the manifest bytes and the files, which are named by the
// slash separated paths the manifest uses. It keeps the map it is given, so a
// caller must not change it afterwards
func NewMemoryAssets(manifest []byte, files map[string][]byte) *MemoryAssets {
	return &MemoryAssets{manifest: manifest, files: files}
}

// ReadManifest returns the manifest bytes
func (m *MemoryAssets) ReadManifest() ([]byte, error) { return m.manifest, nil }

// Open returns a reader for the file at rel, and fails for a path that is not held
// or that would leave the root
func (m *MemoryAssets) Open(rel string) (io.ReadCloser, error) {
	if clean := path.Clean(rel); clean != rel || path.IsAbs(rel) || rel == ".." || len(rel) > 2 && rel[:3] == "../" {
		return nil, fmt.Errorf("template: path %q is not inside the template", rel)
	}
	b, ok := m.files[rel]
	if !ok {
		return nil, fmt.Errorf("template: no file %q", rel)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
