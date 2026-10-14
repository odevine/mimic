package studio

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/ui/internal/apierr"
)

// How a material of a part is covered
const (
	// StateImage is a material with an image of its own
	StateImage = "image"
	// StateBlended is a run of colors drawn by blending the images of each color
	StateBlended = "blended"
	// StateAny is a material drawn with the part's "any" image
	StateAny = "any"
	// StateMissing is a material nothing can draw, so a card of it leaves a hole
	StateMissing = "missing"
	// StateExtra is an image whose name is not one the part's slot reads
	StateExtra = "extra"
)

// MaterialView is one material of a part and the image that draws it
type MaterialView struct {
	Key   string `json:"key"`
	State string `json:"state"`
	// File is the image's path in the draft, and Width, Height and Bytes say what it
	// is. They are empty for a material with no image of its own
	File   string `json:"file,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
	// From lists the materials a blended run is made from
	From []string `json:"from,omitempty"`
}

// PartView is a part of the template with a cell for each material its slot reads
type PartView struct {
	Name      string         `json:"name"`
	Slot      string         `json:"slot,omitempty"`
	Dir       string         `json:"dir"`
	Materials []MaterialView `json:"materials"`
	// Problems are what is wrong with the part's images, such as sizes that differ
	Problems []string `json:"problems,omitempty"`
}

// FontView is a font the template ships
type FontView struct {
	ID      string `json:"id"`
	File    string `json:"file"`
	Family  string `json:"family,omitempty"`
	Style   string `json:"style,omitempty"`
	Source  string `json:"source,omitempty"`
	License string `json:"license,omitempty"`
	// Missing says the file is not in the draft
	Missing bool `json:"missing,omitempty"`
}

// RoleView is a text role and the font it draws with. Font is the id of one of the
// template's fonts and Builtin the id of one the engine ships, and when neither is
// set by the template the role draws with its default, Default
type RoleView struct {
	Role     string `json:"role"`
	Label    string `json:"label"`
	Standard bool   `json:"standard"`
	Font     string `json:"font,omitempty"`
	Builtin  string `json:"builtin,omitempty"`
	Default  string `json:"default,omitempty"`
}

// BuiltinView is a font that ships with the engine
type BuiltinView struct {
	ID string `json:"id"`
}

// AssetsView is the images and fonts a draft holds, as the editor shows them
type AssetsView struct {
	Parts    []PartView    `json:"parts"`
	Fonts    []FontView    `json:"fonts"`
	Roles    []RoleView    `json:"roles"`
	Builtins []BuiltinView `json:"builtins"`
	// Slots is the slot each part can read, with the materials each one has an image
	// for
	Slots []SlotView `json:"slots"`
}

// SlotView is a slot and the materials it reads
type SlotView struct {
	Name      string   `json:"name"`
	Materials []string `json:"materials"`
}

const singles = "wubrg"

// readSource reads template.json as the packer does, or reports that it cannot
func readSource(fsys fs.FS) (*author.Source, error) {
	raw, err := fs.ReadFile(fsys, "template.json")
	if err != nil {
		return nil, err
	}
	var src author.Source
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, err
	}
	return &src, nil
}

// partDir is the folder a part's images are in, which is the part's own name when it
// names none
func partDir(name string, p author.Part) string {
	if p.Dir != "" {
		return p.Dir
	}
	return name
}

// partFiles lists a part's images by material, from its folder and then its own
// variants, as paths in the draft
func partFiles(fsys fs.FS, name string, p author.Part) map[string]string {
	out := map[string]string{}
	if entries, err := fs.ReadDir(fsys, partDir(name, p)); err == nil {
		for _, e := range entries {
			if stem, ok := strings.CutSuffix(e.Name(), ".png"); ok && !e.IsDir() {
				out[stem] = path.Join(partDir(name, p), e.Name())
			}
		}
	}
	for key, file := range p.Variants {
		out[key] = file
	}
	return out
}

// Assets lists the draft's parts, each with every material its slot reads and what
// covers it, and its fonts and the roles they are assigned to
func (s *Service) Assets(id string) (AssetsView, error) {
	fsys, err := s.store.FS(id)
	if err != nil {
		return AssetsView{}, err
	}
	view := AssetsView{Parts: []PartView{}, Fonts: []FontView{}, Roles: []RoleView{}, Builtins: []BuiltinView{}, Slots: slotViews()}
	for _, b := range fonts.Builtins() {
		view.Builtins = append(view.Builtins, BuiltinView{ID: b.ID})
	}
	src, err := readSource(fsys)
	if err != nil {
		view.Roles = roleViews(nil)
		return view, nil
	}
	names := make([]string, 0, len(src.Parts))
	for name := range src.Parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		view.Parts = append(view.Parts, partView(fsys, name, src.Parts[name]))
	}
	ids := make([]string, 0, len(src.Fonts))
	for fid := range src.Fonts {
		ids = append(ids, fid)
	}
	sort.Strings(ids)
	for _, fid := range ids {
		f := src.Fonts[fid]
		_, statErr := fs.Stat(fsys, f.File)
		view.Fonts = append(view.Fonts, FontView{ID: fid, File: f.File, Family: f.Family, Style: f.Style, Source: f.Source, License: f.License, Missing: statErr != nil})
	}
	view.Roles = roleViews(src.Roles)
	return view, nil
}

func slotViews() []SlotView {
	out := []SlotView{{Name: "", Materials: manifest.VariantKeys(frame.SlotNone)}}
	for _, name := range frame.SlotNames() {
		slot, err := frame.ParseSlot(name)
		if err != nil {
			continue
		}
		out = append(out, SlotView{Name: name, Materials: manifest.VariantKeys(slot)})
	}
	return out
}

// roleViews lists the standard roles first, with the font each draws with, and then
// the roles the template defines
func roleViews(defined map[string]manifest.FontRole) []RoleView {
	var out []RoleView
	seen := map[string]bool{}
	var catalog *fonts.Catalog
	for _, info := range catalog.Roles() {
		v := RoleView{Role: info.Name, Label: info.Label, Standard: true, Default: info.Builtin}
		if d, ok := defined[info.Name]; ok {
			v.Font, v.Builtin = d.Font, d.Builtin
			if d.Label != "" {
				v.Label = d.Label
			}
		}
		seen[info.Name] = true
		out = append(out, v)
	}
	var custom []string
	for name := range defined {
		if !seen[name] {
			custom = append(custom, name)
		}
	}
	sort.Strings(custom)
	for _, name := range custom {
		d := defined[name]
		label := d.Label
		if label == "" {
			label = name
		}
		out = append(out, RoleView{Role: name, Label: label, Font: d.Font, Builtin: d.Builtin})
	}
	return out
}

func partView(fsys fs.FS, name string, p author.Part) PartView {
	slot, err := frame.ParseSlot(p.Slot)
	if err != nil {
		slot = frame.SlotNone
	}
	v := PartView{Name: name, Slot: p.Slot, Dir: partDir(name, p), Materials: []MaterialView{}}
	files := partFiles(fsys, name, p)
	keys := manifest.VariantKeys(slot)
	known := map[string]bool{}
	for _, k := range keys {
		known[k] = true
	}
	_, hasAny := files["any"]
	sizes := map[image.Point][]string{}
	view := func(key, state, file string) MaterialView {
		m := MaterialView{Key: key, State: state, File: file}
		if file == "" {
			return m
		}
		if st, err := fs.Stat(fsys, file); err == nil {
			m.Bytes = st.Size()
		}
		if f, err := fsys.Open(file); err == nil {
			if cfg, err := png.DecodeConfig(f); err == nil {
				m.Width, m.Height = cfg.Width, cfg.Height
				sizes[image.Pt(cfg.Width, cfg.Height)] = append(sizes[image.Pt(cfg.Width, cfg.Height)], key)
			}
			f.Close()
		}
		return m
	}
	for _, key := range keys {
		file, has := files[key]
		switch {
		case has:
			v.Materials = append(v.Materials, view(key, StateImage, file))
		case isRun(key) && coveredBy(key, files):
			m := MaterialView{Key: key, State: StateBlended}
			for _, r := range key {
				m.From = append(m.From, string(r))
			}
			v.Materials = append(v.Materials, m)
		case hasAny && key != "any":
			v.Materials = append(v.Materials, MaterialView{Key: key, State: StateAny})
		default:
			v.Materials = append(v.Materials, MaterialView{Key: key, State: StateMissing})
		}
	}
	var extra []string
	for key := range files {
		if !known[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		v.Materials = append(v.Materials, view(key, StateExtra, files[key]))
	}
	if len(sizes) > 1 {
		var parts []string
		for size, keys := range sizes {
			sort.Strings(keys)
			parts = append(parts, fmt.Sprintf("%dx%d for %s", size.X, size.Y, strings.Join(keys, ", ")))
		}
		sort.Strings(parts)
		v.Problems = append(v.Problems, "The images are not one size: "+strings.Join(parts, "; "))
	}
	return v
}

// isRun says a key is a run of two or more colors, which draws by blending the
// image of each color
func isRun(key string) bool {
	if len(key) < 2 {
		return false
	}
	for _, r := range key {
		if !strings.ContainsRune(singles, r) {
			return false
		}
	}
	return true
}

func coveredBy(key string, files map[string]string) bool {
	for _, r := range key {
		if _, ok := files[string(r)]; !ok {
			return false
		}
	}
	return true
}

// ThumbRequest names images of a draft to be drawn small
type ThumbRequest struct {
	ID    string   `json:"id"`
	Files []string `json:"files"`
	// Edge is the longest side of a thumbnail in pixels
	Edge int `json:"edge"`
}

const (
	maxThumbFiles = 64
	maxThumbEdge  = 512
)

// Thumbs draws images of the draft small, by their path, and returns each as a
// data URL. An image that cannot be read is left out
func (s *Service) Thumbs(req ThumbRequest) (map[string]string, error) {
	fsys, err := s.store.FS(req.ID)
	if err != nil {
		return nil, err
	}
	if len(req.Files) > maxThumbFiles {
		return nil, apierr.Newf(apierr.BadRequest, "Ask for at most %d thumbnails at once", maxThumbFiles)
	}
	edge := min(max(req.Edge, 16), maxThumbEdge)
	out := make(map[string]string, len(req.Files))
	for _, file := range req.Files {
		if !fs.ValidPath(file) || strings.ToLower(path.Ext(file)) != ".png" {
			return nil, apierr.Newf(apierr.BadRequest, "%q is not an image of the template", file)
		}
		key := fmt.Sprintf("%s/%s@%d", req.ID, file, edge)
		st, err := fs.Stat(fsys, file)
		if err != nil {
			continue
		}
		stamp := fmt.Sprintf("%d-%d", st.Size(), st.ModTime().UnixNano())
		if url, ok := s.thumbs.get(key, stamp); ok {
			out[file] = url
			continue
		}
		f, err := fsys.Open(file)
		if err != nil {
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, shrink(img, edge)); err != nil {
			continue
		}
		url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
		s.thumbs.put(key, stamp, url)
		out[file] = url
	}
	return out, nil
}

// shrink scales an image down so its longest side is edge pixels, by averaging the
// source pixels each new one covers, with the color of a transparent pixel weighted
// by its alpha. An image already that small is returned as it is
func shrink(src image.Image, edge int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= edge && h <= edge {
		return src
	}
	nw, nh := edge, edge
	if w >= h {
		nh = max(1, h*edge/w)
	} else {
		nw = max(1, w*edge/h)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := b.Min.Y+y*h/nh, b.Min.Y+max((y+1)*h/nh, y*h/nh+1)
		for x := 0; x < nw; x++ {
			x0, x1 := b.Min.X+x*w/nw, b.Min.X+max((x+1)*w/nw, x*w/nw+1)
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					pr, pg, pb, pa := src.At(xx, yy).RGBA()
					r, g, bl, a, n = r+uint64(pr), g+uint64(pg), bl+uint64(pb), a+uint64(pa), n+1
				}
			}
			if a == 0 || n == 0 {
				continue
			}
			// The sums are premultiplied, so dividing by the alpha gives the color
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r * 255 / a), G: uint8(g * 255 / a), B: uint8(bl * 255 / a), A: uint8(a / n >> 8)})
		}
	}
	return dst
}

// thumbCache keeps drawn thumbnails, which are small, by the image's size and time
// so a changed image is drawn again
type thumbCache struct {
	mu sync.Mutex
	m  map[string]thumb
}

type thumb struct{ stamp, url string }

func (c *thumbCache) get(key, stamp string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.m[key]
	return t.url, ok && t.stamp == stamp
}

func (c *thumbCache) put(key, stamp, url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) > 512 {
		c.m = map[string]thumb{}
	}
	c.m[key] = thumb{stamp, url}
}
