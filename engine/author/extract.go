package author

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

// ExtractOptions says what to write into template.json for the fields a manifest
// does not hold, which a bundle keeps in its header
type ExtractOptions struct {
	// Version is the template's version. Empty is 0.1.0
	Version string
	// MinEngine is the first engine release that reads the template
	MinEngine string
}

// Extract turns a template in its resolved form back into a source folder, the
// files an author edits, and returns each by its slash separated path. The folder
// builds to the template it was read from: each part's images go in a folder of the
// part's name under "parts/", each layout is written complete in "layouts/", and
// fonts stay at the path the manifest gives them. Inheritance is not recovered,
// because the resolved form does not keep it
func Extract(p template.AssetProvider, opt ExtractOptions) (map[string][]byte, manifest.Diagnostics, error) {
	raw, err := p.ReadManifest()
	if err != nil {
		return nil, nil, err
	}
	m, ds := manifest.Decode(raw)
	if err := ds.Err(); err != nil {
		return nil, ds, err
	}
	files := map[string][]byte{}
	src := Source{
		Contract: m.Contract, ID: m.ID, Version: opt.Version, MinEngine: opt.MinEngine,
		Name: m.Name, Description: m.Description, Author: m.Author, License: m.License,
		Credits: m.Credits, Features: m.Features, Parts: map[string]Part{},
		Fonts: m.Fonts, Roles: m.Roles,
	}
	if src.Version == "" {
		src.Version = "0.1.0"
	}

	for _, name := range sortedKeys(m.Parts) {
		part := m.Parts[name]
		dir := path.Join("parts", name)
		for _, key := range sortedKeys(part.Variants) {
			data, err := readAsset(p, part.Variants[key].Path)
			if err != nil {
				return nil, ds, fmt.Errorf("author: the image for part %q, %q: %w", name, key, err)
			}
			files[path.Join(dir, key+".png")] = data
		}
		src.Parts[name] = Part{Dir: dir, Slot: part.Slot, Origin: part.Origin, Blend: part.Blend}
	}
	for _, f := range m.Fonts {
		data, err := readAsset(p, f.File)
		if err != nil {
			return nil, ds, fmt.Errorf("author: the font %s: %w", f.File, err)
		}
		files[f.File] = data
	}
	for _, name := range sortedKeys(m.Layouts) {
		if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
			return nil, ds, fmt.Errorf("author: %q is not a layout name that can be a file", name)
		}
		data, err := json.MarshalIndent(m.Layouts[name], "", "  ")
		if err != nil {
			return nil, ds, fmt.Errorf("author: layout %q: %w", name, err)
		}
		files[path.Join(layoutsDir, name+".json")] = append(data, '\n')
	}
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return nil, ds, err
	}
	files[sourceFile] = append(data, '\n')
	return files, ds, nil
}

// readAsset reads a file a manifest names, up to the size of an image
func readAsset(p template.AssetProvider, rel string) ([]byte, error) {
	rc, err := p.Open(rel)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("%s is over the limit of %d bytes", rel, maxImageBytes)
	}
	return data, nil
}
