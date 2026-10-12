// Package templatetest builds small templates for tests, as loose folders and as
// bundles, so the pieces of the app that install and render templates can be
// driven offline. A template is flat colors where a frame's art goes, built
// through the engine's own packer so it is what a real template looks like to the
// app
package templatetest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing/fstest"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

// Canvas is the size of every template's canvas
const (
	Width  = 744
	Height = 1039
)

// Spec describes a template to build
type Spec struct {
	// ID is the template's id. Empty is "test.frame"
	ID string
	// Version is its version. Empty is "0.1.0"
	Version string
	// Name is what it is called to a person. Empty is "Test frame"
	Name string
	// Shapes are the faces it renders. Empty is a standard single card
	Shapes []frame.Shape
	// Features are the choices it offers
	Features map[string]manifest.Feature
	// MinEngine is the oldest engine that renders it. Empty is none
	MinEngine string
}

// Features is a feature of each type. The banner draws a strip across the top of
// the card when it is on, which the other two do not change
var Features = map[string]manifest.Feature{
	"banner":   {Type: "bool", Label: "Banner"},
	"tone":     {Type: "choice", Label: "Tone", Values: []string{"light", "dark"}, Default: json.RawMessage(`"light"`)},
	"strength": {Type: "range", Label: "Strength", Min: 0, Max: 1, Step: 0.25, Default: json.RawMessage(`0.5`)},
}

// Single is the shape of a standard single card
var Single = frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard}

// Transform are the shapes of the two faces of a transform card
var Transform = []frame.Shape{
	{Role: frame.RoleTransformFront, Kind: frame.KindStandard},
	{Role: frame.RoleTransformBack, Kind: frame.KindStandard},
}

func (s Spec) filled() Spec {
	if s.ID == "" {
		s.ID = "test.frame"
	}
	if s.Version == "" {
		s.Version = "0.1.0"
	}
	if s.Name == "" {
		s.Name = "Test frame"
	}
	if len(s.Shapes) == 0 {
		s.Shapes = []frame.Shape{Single}
	}
	return s
}

// colors fills the background of each material
var colors = map[string]color.NRGBA{
	"w": {0xF8, 0xF6, 0xD8, 0xFF}, "u": {0xB5, 0xD7, 0xE9, 0xFF}, "b": {0xB6, 0xAE, 0xA8, 0xFF},
	"r": {0xE7, 0xA9, 0x8B, 0xFF}, "g": {0xA3, 0xC9, 0xA8, 0xFF}, "gold": {0xD6, 0xBF, 0x6E, 0xFF},
	"artifact": {0xB9, 0xC2, 0xC9, 0xFF}, "colorless": {0xB9, 0xC2, 0xC9, 0xFF},
	"land": {0xC3, 0xA8, 0x7F, 0xFF}, "vehicle": {0xB9, 0xC2, 0xC9, 0xFF},
}

func flat(rect image.Rectangle, fill color.NRGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (s Spec) source() fstest.MapFS {
	s = s.filled()
	fsys := fstest.MapFS{}
	full := image.Rect(0, 0, Width, Height)
	for key, c := range colors {
		fsys["bg/"+key+".png"] = &fstest.MapFile{Data: flat(full, c)}
	}
	fsys["textbox.png"] = &fstest.MapFile{Data: flat(image.Rect(48, 612, 696, 984), color.NRGBA{0xF4, 0xF1, 0xE6, 0xDC})}
	fsys["crown.png"] = &fstest.MapFile{Data: flat(image.Rect(36, 30, 708, 120), color.NRGBA{0xA0, 0x7E, 0x2A, 0xFF})}

	tmpl := map[string]any{
		"contract": manifest.Contract, "id": s.ID, "version": s.Version, "name": s.Name,
		"description": "Flat colors where a frame's art goes",
		"parts": map[string]any{
			"background": map[string]any{"slot": "background", "dir": "bg"},
			"textbox":    map[string]any{"variants": map[string]string{"any": "textbox.png"}},
			"crown":      map[string]any{"variants": map[string]string{"any": "crown.png"}},
		},
	}
	if s.MinEngine != "" {
		tmpl["minEngine"] = s.MinEngine
	}
	if len(s.Features) > 0 {
		tmpl["features"] = s.Features
	}
	box := func(x, y, w, h int, size float64, font, align string) map[string]any {
		return map[string]any{
			"rect": map[string]any{"x": x, "y": y, "width": w, "height": h},
			"text": map[string]any{"fontSize": size, "font": font, "color": "#111111", "align": align},
		}
	}
	title := box(48, 40, 520, 52, 38, "title", "left")
	title["flow"] = map[string]any{"clearOf": "mana"}
	mana := box(500, 46, 208, 44, 28, "", "right")
	mana["text"].(map[string]any)["minFontSize"] = 28
	stack := []any{
		map[string]any{"part": "background"},
		map[string]any{"part": "textbox"},
		map[string]any{"part": "crown", "condition": "legendary"},
	}
	if _, ok := s.Features["banner"]; ok {
		stack = append(stack, map[string]any{"id": "banner", "part": "crown", "condition": "feature:banner"})
	}
	layout := map[string]any{
		"serves": s.Shapes,
		"canvas": map[string]any{"width": Width, "height": Height},
		"stack":  stack,
		"textBoxes": map[string]any{
			"title": title, "mana": mana,
			"type":   box(48, 566, 648, 34, 24, "title", "left"),
			"oracle": box(72, 636, 600, 320, 24, "", "left"),
			"pt":     box(596, 946, 112, 56, 34, "title", "center"),
		},
		"art": []any{map[string]any{"x": 60, "y": 132, "width": 624, "height": 424, "after": "background"}},
	}
	for name, v := range map[string]any{"template.json": tmpl, "layouts/main.json": layout} {
		raw, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		fsys[name] = &fstest.MapFile{Data: raw}
	}
	return fsys
}

var built = struct {
	sync.Mutex
	byKey map[string]*author.Built
}{byKey: map[string]*author.Built{}}

// Pack packs the template, once for each distinct spec in a process, since
// encoding its images is the slow part
func (s Spec) Pack() (*author.Built, error) {
	s = s.filled()
	key := fmt.Sprintf("%s|%s|%s|%v|%v|%s", s.ID, s.Version, s.Name, s.Shapes, s.Features, s.MinEngine)
	built.Lock()
	defer built.Unlock()
	if b, ok := built.byKey[key]; ok {
		return b, nil
	}
	b, ds := author.Build(s.source(), author.Options{})
	if b == nil {
		return nil, fmt.Errorf("templatetest: building %s: %v", s.ID, ds)
	}
	built.byKey[key] = b
	return b, nil
}

// Zip is the template as a .mimic and its SHA-256
func (s Spec) Zip() ([]byte, string, error) {
	b, err := s.Pack()
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	if err := b.WriteZip(&buf); err != nil {
		return nil, "", fmt.Errorf("templatetest: writing the bundle: %w", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}

// Write writes the template into dir as a loose developer folder, the resolved
// form with a manifest.json and its images
func (s Spec) Write(dir string) error {
	b, err := s.Pack()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b.ManifestJSON, 0o644); err != nil {
		return err
	}
	paths := make([]string, 0, len(b.Assets))
	for p := range b.Assets {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b.Assets[p], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// T is what the helpers need from a test, which *testing.T satisfies
type T interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Bundle is Zip for a test, and fails it on an error
func (s Spec) Bundle(t T) ([]byte, string) {
	t.Helper()
	data, sha, err := s.Zip()
	if err != nil {
		t.Fatalf("%v", err)
	}
	return data, sha
}

// WriteLoose is Write for a test, and fails it on an error
func (s Spec) WriteLoose(t T, dir string) {
	t.Helper()
	if err := s.Write(dir); err != nil {
		t.Fatalf("%v", err)
	}
}

// Dir is the folder name a loose template is written to, the id's name after the owner
func (s Spec) Dir() string {
	s = s.filled()
	_, name, _ := strings.Cut(s.ID, ".")
	return name
}
