// Package author reads a template in its source form, a folder an author edits,
// and builds the resolved form a bundle holds. The source form names a part by the
// directory of its images and lets a layout extend others, and building resolves
// both, cuts each part to the pixels it uses, names every image by the hash of its
// content, and writes a zip that is the same bytes for the same folder
package author

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/odevine/mimic/engine/manifest"
)

// Source is template.json, the part of a template's source form that is not a
// layout. Layouts are the files in the layouts folder, one for each, named by the
// file
type Source struct {
	Contract    string            `json:"contract"`
	ID          string            `json:"id"`
	Version     string            `json:"version"`
	MinEngine   string            `json:"minEngine,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Author      manifest.Author   `json:"author,omitempty"`
	License     string            `json:"license,omitempty"`
	Credits     []manifest.Credit `json:"credits,omitempty"`

	Features map[string]manifest.Feature `json:"features,omitempty"`
	Parts    map[string]Part             `json:"parts"`
}

// Part is an art set in the source form
type Part struct {
	// Dir is the folder of the part's images, whose file names without the
	// extension are the materials they are for. Empty is a folder with the part's
	// own name, when there is one
	Dir string `json:"dir,omitempty"`
	// Variants adds an image, or replaces the one a folder gives, for a material, by
	// its path from the template's folder
	Variants map[string]string `json:"variants,omitempty"`
	Slot     string            `json:"slot,omitempty"`
	// Origin is where the images' top left sits in the canvas before they are cut.
	// Empty is the canvas's corner, for an image the size of the canvas
	Origin manifest.Point  `json:"origin,omitempty"`
	Blend  *manifest.Blend `json:"blend,omitempty"`
}

const (
	sourceFile = "template.json"
	layoutsDir = "layouts"
)

// maxFile bounds what the reader takes from a source file that is not an image
const maxFile = manifest.MaxBytes

// readSource reads template.json
func readSource(fsys fs.FS) (*Source, manifest.Diagnostics) {
	raw, err := readLimited(fsys, sourceFile)
	if err != nil {
		return nil, manifest.Diagnostics{{Severity: manifest.Error, Path: sourceFile, Message: err.Error()}}
	}
	var s Source
	if err := decodeStrict(raw, &s); err != nil {
		return nil, manifest.Diagnostics{{Severity: manifest.Error, Path: sourceFile, Message: err.Error()}}
	}
	return &s, nil
}

// readLayouts reads every file in the layouts folder as a raw layout, by name
func readLayouts(fsys fs.FS) (map[string]rawLayout, manifest.Diagnostics) {
	var ds manifest.Diagnostics
	entries, err := fs.ReadDir(fsys, layoutsDir)
	if err != nil {
		return nil, append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: layoutsDir, Message: "the template has no layouts folder"})
	}
	out := map[string]rawLayout{}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		file := path.Join(layoutsDir, e.Name())
		if e.IsDir() || !ok {
			ds = append(ds, manifest.Diagnostic{Severity: manifest.Warning, Path: file, Message: "is not a layout file, which is a .json file, so it is ignored"})
			continue
		}
		raw, err := readLimited(fsys, file)
		if err != nil {
			ds = append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: file, Message: err.Error()})
			continue
		}
		var l rawLayout
		if err := json.Unmarshal(raw, &l); err != nil {
			ds = append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: file, Message: err.Error()})
			continue
		}
		out[name] = l
	}
	if len(out) == 0 && !ds.HasErrors() {
		ds = append(ds, manifest.Diagnostic{Severity: manifest.Error, Path: layoutsDir, Message: "the layouts folder holds no layout"})
	}
	return out, ds
}

func readLimited(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxFile {
		return nil, fmt.Errorf("is over the limit of %d bytes", maxFile)
	}
	return raw, nil
}

// decodeStrict reads one JSON value and refuses a field the type does not have
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("has more than one value")
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
