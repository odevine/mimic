package author

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

// maxImageBytes bounds one source image's file
const maxImageBytes = 256 << 20

// expandPart lists the images of a part by material, from its folder and then its
// own variants, as paths from the template's folder
func expandPart(fsys fs.FS, name string, p Part) (map[string]string, manifest.Diagnostics) {
	at := "parts." + name
	var ds manifest.Diagnostics
	out := map[string]string{}
	dir, explicit := p.Dir, p.Dir != ""
	if !explicit {
		dir = name
	}
	if dir != path.Clean(dir) || path.IsAbs(dir) || dir == ".." || strings.HasPrefix(dir, "../") {
		return nil, append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: at + ".dir", Message: fmt.Sprintf("%q is not a folder inside the template", dir)})
	}
	entries, err := fs.ReadDir(fsys, dir)
	switch {
	case err == nil:
		for _, e := range entries {
			stem, ok := strings.CutSuffix(e.Name(), ".png")
			if e.IsDir() || !ok {
				ds = append(ds, manifest.Diagnostic{Severity: manifest.Warning, Path: at, Message: fmt.Sprintf("%s is not a .png image, so it is ignored", path.Join(dir, e.Name()))})
				continue
			}
			out[stem] = path.Join(dir, e.Name())
		}
	case explicit || len(p.Variants) == 0:
		ds = append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: at + ".dir", Message: fmt.Sprintf("the folder %q cannot be read: %v", dir, err)})
	}
	for key, v := range p.Variants {
		out[key] = v
	}
	return out, ds
}

// assetStore collects the images of a bundle by the hash of their content
type assetStore struct {
	mu    sync.Mutex
	files map[string][]byte
	// check is set for a build that only validates, which stores nothing
	check bool
}

// put keeps an image and returns the path it is named by
func (s *assetStore) put(data []byte) string {
	sum := sha256.Sum256(data)
	name := "assets/" + hex.EncodeToString(sum[:8]) + ".png"
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files == nil {
		s.files = map[string][]byte{}
	}
	if _, ok := s.files[name]; !ok {
		s.files[name] = data
	}
	return name
}

// putAt keeps a file that is named by its own path, such as a font
func (s *assetStore) putAt(name string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files == nil {
		s.files = map[string][]byte{}
	}
	s.files[name] = data
}

// packPart cuts a part's images to the rectangle that holds the visible pixels of
// all of them and stores them. It returns the part with the new paths and origin,
// and the offset of the cut, which an entry that places the part itself needs. The
// images stay as they are when nothing is cut, so a part drawn to size costs no
// encoding
func packPart(fsys fs.FS, name string, sources map[string]string, p manifest.Part, store *assetStore) (manifest.Part, image.Point, manifest.Diagnostics) {
	at := "parts." + name
	fail := func(format string, args ...any) (manifest.Part, image.Point, manifest.Diagnostics) {
		return p, image.Point{}, manifest.Diagnostics{{Severity: manifest.Error, Path: at, Message: fmt.Sprintf(format, args...)}}
	}
	keys := sortedKeys(sources)
	raws := make(map[string][]byte, len(keys))
	var size image.Point
	var union image.Rectangle
	for _, key := range keys {
		raw, err := readImage(fsys, sources[key])
		if err != nil {
			return fail("the image for %q, %s, cannot be read: %v", key, sources[key], err)
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			return fail("the image for %q, %s, is not a PNG: %v", key, sources[key], err)
		}
		b := img.Bounds()
		if size == (image.Point{}) {
			size = b.Size()
		} else if b.Size() != size {
			return fail("the image for %q is %dx%d and the others are %dx%d, and a part's images must be one size", key, b.Dx(), b.Dy(), size.X, size.Y)
		}
		union = union.Union(template.OpaqueBounds(img))
		raws[key] = raw
	}
	if union.Empty() {
		return fail("none of the images has a visible pixel")
	}
	cut := union != image.Rect(0, 0, size.X, size.Y) && !store.check
	out := p
	out.Variants = make(map[string]manifest.Variant, len(keys))
	for _, key := range keys {
		data := raws[key]
		if cut {
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				return fail("the image for %q is not a PNG: %v", key, err)
			}
			data, err = encodeCut(img, union)
			if err != nil {
				return fail("the image for %q cannot be cut: %v", key, err)
			}
		}
		if store.check {
			continue
		}
		out.Variants[key] = manifest.Variant{Path: store.put(data)}
	}
	off := union.Min
	out.Origin = manifest.Point{X: p.Origin.X + off.X, Y: p.Origin.Y + off.Y}
	return out, off, nil
}

func readImage(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxImageBytes {
		return nil, fmt.Errorf("is over the limit of %d bytes", maxImageBytes)
	}
	var buf bytes.Buffer
	buf.Grow(int(info.Size()))
	if _, err := buf.ReadFrom(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodeCut encodes the rectangle r of img as a PNG
func encodeCut(img image.Image, r image.Rectangle) ([]byte, error) {
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		nrgba = image.NewNRGBA(img.Bounds())
		draw.Draw(nrgba, nrgba.Bounds(), img, img.Bounds().Min, draw.Src)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, nrgba.SubImage(r)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sortDiagnostics(ds manifest.Diagnostics) {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].Path != ds[j].Path {
			return ds[i].Path < ds[j].Path
		}
		return ds[i].Message < ds[j].Message
	})
}
