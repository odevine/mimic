package template

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/odevine/mimic/engine/version"
)

// bundleFormat is the .mimic container schema this reader understands.
// bundle.json carries the format a bundle was written with, and any other one
// means the structure differs and this engine cannot read it
const bundleFormat = 2

// bundleHeader is bundle.json, read first so an incompatible container is
// rejected before the larger manifest or any layer is touched
type bundleHeader struct {
	Format    int    `json:"format"`
	ID        string `json:"id"`
	Version   string `json:"version"`
	MinEngine string `json:"minEngine"`
}

// ZipAssetProvider reads a template's manifest and layer PNGs from a .mimic
// bundle, the zip archive the templates repo publishes. It satisfies the same
// AssetProvider interface as FSAssetProvider, so render.go and every template
// package are identical whether assets come from a loose directory or a bundle.
// The bundle stays open for the provider's lifetime, so callers Close it when
// done
type ZipAssetProvider struct {
	f      *os.File
	zr     *zip.Reader
	byName map[string]*zip.File
	header bundleHeader
}

// NewZipAssetProvider opens a .mimic at path and gates it once: it reads
// bundle.json and rejects a container newer than this reader understands or one
// whose minEngine exceeds the running engine, so an incompatible bundle fails
// here rather than deep in a render. The file is held open for random access
// through the zip central directory, so a single layer opens without reading
// the rest
func NewZipAssetProvider(path string) (*ZipAssetProvider, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("template: opening bundle: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("template: stat bundle: %w", err)
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("template: reading %q as a bundle: %w", path, err)
	}

	p := &ZipAssetProvider{f: f, zr: zr, byName: make(map[string]*zip.File, len(zr.File))}
	for _, entry := range zr.File {
		p.byName[entry.Name] = entry
	}

	if err := p.readHeader(); err != nil {
		f.Close()
		return nil, err
	}
	return p, nil
}

// readHeader decodes bundle.json and applies the format and minEngine gates
func (p *ZipAssetProvider) readHeader() error {
	raw, err := p.read("bundle.json")
	if err != nil {
		return fmt.Errorf("template: reading bundle header: %w", err)
	}
	if err := json.Unmarshal(raw, &p.header); err != nil {
		return fmt.Errorf("template: decoding bundle header: %w", err)
	}
	if p.header.Format != bundleFormat {
		return fmt.Errorf("template: bundle format %d is not one this engine reads, which reads format %d",
			p.header.Format, bundleFormat)
	}
	if !version.Satisfies(p.header.MinEngine) {
		return fmt.Errorf("template: bundle needs engine %s or newer, running %s; update the engine",
			p.header.MinEngine, version.Version)
	}
	return nil
}

// ReadManifest reads manifest.json, which is no larger than a manifest may be
func (p *ZipAssetProvider) ReadManifest() ([]byte, error) {
	entry, ok := p.byName["manifest.json"]
	if !ok {
		return nil, fmt.Errorf("template: reading manifest: \"manifest.json\" not found in bundle")
	}
	rc, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("template: reading manifest: %w", err)
	}
	defer rc.Close()
	return readManifest(rc)
}

// Open returns a reader for a layer PNG at relPath, guarded against a name that
// escapes the bundle's tree the same way FSAssetProvider guards a directory
func (p *ZipAssetProvider) Open(relPath string) (io.ReadCloser, error) {
	name, err := safeRelPath(relPath)
	if err != nil {
		return nil, err
	}
	entry, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("template: %q not found in bundle", name)
	}
	return entry.Open()
}

// Close releases the underlying bundle file. After Close the provider must not
// be used
func (p *ZipAssetProvider) Close() error {
	return p.f.Close()
}

// read returns the full bytes of one entry by exact name, used for the small
// JSON entries the provider owns
func (p *ZipAssetProvider) read(name string) ([]byte, error) {
	entry, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("%q not found in bundle", name)
	}
	rc, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
