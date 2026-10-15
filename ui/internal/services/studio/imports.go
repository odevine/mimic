package studio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/draftedit"
)

const (
	// maxImageFile and maxImageSide bound an image the editor takes in, which is
	// larger than any card frame at the highest resolution the engine renders
	maxImageFile   = 128 << 20
	maxImageSide   = 16384
	maxImagePixels = 120_000_000
)

// ImportResult is what an import changed, with what is worth knowing about the file
// that came in. ID is the part or font it went into
type ImportResult struct {
	ApplyResult
	ID       string   `json:"id"`
	Warnings []string `json:"warnings"`
}

// ImageRequest is an image file to put in a part of a draft as the image for a
// material. A part that is not in the draft is made, with the slot given
type ImageRequest struct {
	ID       string `json:"id"`
	Part     string `json:"part"`
	Material string `json:"material"`
	Slot     string `json:"slot"`
	Path     string `json:"path"`
}

// partName is what a part may be called, as the engine checks it
var partName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// readFileLimited reads a file the person chose, refusing one over a size
func readFileLimited(file string, limit int64, what string) ([]byte, error) {
	st, err := os.Stat(file)
	if err != nil {
		return nil, apierr.Wrap(apierr.NotFound, "Could not read that file", err)
	}
	if !st.Mode().IsRegular() {
		return nil, apierr.New(apierr.BadRequest, "That is not a file")
	}
	if st.Size() > limit {
		return nil, apierr.Newf(apierr.TooLarge, "That %s is over %d MB", what, limit>>20)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, apierr.Wrap(apierr.NotFound, "Could not read that file", err)
	}
	return raw, nil
}

// ImportImage copies a PNG into a part of the draft as the image for a material,
// writing over the one that was there, so its place and condition stay. The image
// is checked here, and a problem that does not stop it being used comes back as a
// warning
func (s *Service) ImportImage(req ImageRequest) (ImportResult, error) {
	raw, err := readFileLimited(req.Path, maxImageFile, "image")
	if err != nil {
		return ImportResult{}, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return ImportResult{}, apierr.New(apierr.BadRequest, "That is not a PNG image. Export the layer as a PNG")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageSide || cfg.Height > maxImageSide || cfg.Width*cfg.Height > maxImagePixels {
		return ImportResult{}, apierr.Newf(apierr.TooLarge, "That image is %d by %d pixels, and the editor takes images up to %d on a side", cfg.Width, cfg.Height, maxImageSide)
	}
	if !partName.MatchString(req.Part) {
		return ImportResult{}, apierr.Newf(apierr.BadRequest, "%q is not a name for a part, which is lowercase letters, digits, underscores and hyphens", req.Part)
	}
	fsys, err := s.store.FS(req.ID)
	if err != nil {
		return ImportResult{}, err
	}
	src, err := readSource(fsys)
	if err != nil {
		return ImportResult{}, apierr.Wrap(apierr.BadRequest, "The template's own file cannot be read, so fix it in the JSON first", err)
	}
	part, exists := src.Parts[req.Part]
	if !exists {
		part.Slot = req.Slot
	}
	slot, err := frame.ParseSlot(part.Slot)
	if err != nil {
		return ImportResult{}, apierr.Newf(apierr.BadRequest, "%q is not a slot a part can read", part.Slot)
	}
	material := req.Material
	if material == "" {
		material = "any"
	}
	if !contains(manifest.VariantKeys(slot), material) {
		return ImportResult{}, apierr.Newf(apierr.BadRequest, "A part with %s does not read an image for %q", slotPhrase(part.Slot), material)
	}

	file := partFiles(fsys, req.Part, part)[material]
	if file == "" {
		file = path.Join(partDir(req.Part, part), material+".png")
	}
	var ops []draftedit.Op
	if !exists {
		value := map[string]string{"dir": "parts/" + req.Part}
		if req.Slot != "" {
			value["slot"] = req.Slot
		}
		blob, _ := json.Marshal(value)
		ops = append(ops, draftedit.Op{Op: draftedit.OpSet, Kind: draftedit.KindTemplate, Path: []string{"parts", req.Part}, Value: blob})
		file = path.Join("parts", req.Part, material+".png")
	}
	ops = append(ops, draftedit.Op{Op: draftedit.OpPut, File: file, Stash: s.held.Put(raw)})

	res, err := s.Apply(ApplyRequest{ID: req.ID, Ops: ops})
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{ApplyResult: res, ID: req.Part, Warnings: s.imageWarnings(fsys, req, part, material, raw, cfg)}, nil
}

func slotPhrase(slot string) string {
	if slot == "" {
		return "no slot"
	}
	return "the " + slot + " slot"
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// imageWarnings says what is worth knowing about an image that was accepted
func (s *Service) imageWarnings(fsys fs.FS, req ImageRequest, part author.Part, material string, raw []byte, cfg image.Config) []string {
	var warns []string
	size := image.Pt(cfg.Width, cfg.Height)
	others := map[image.Point]bool{}
	for key, file := range partFiles(fsys, req.Part, part) {
		if key == material {
			continue
		}
		if f, err := fsys.Open(file); err == nil {
			if c, err := png.DecodeConfig(f); err == nil {
				others[image.Pt(c.Width, c.Height)] = true
			}
			f.Close()
		}
	}
	for o := range others {
		if o != size {
			warns = append(warns, fmt.Sprintf("This image is %dx%d and the part's other images are %dx%d. A part's images must be one size", size.X, size.Y, o.X, o.Y))
			break
		}
	}
	if t, _, err := author.NewDraft(fsys, 0).Open(author.Options{}); err == nil && t != nil {
		for name, l := range t.Manifest().Layouts {
			if c := l.Canvas; c.Width > 0 && (size.X > c.Width || size.Y > c.Height) {
				warns = append(warns, fmt.Sprintf("This image is larger than the %s layout's canvas of %dx%d, so part of it is cut off", name, c.Width, c.Height))
				break
			}
		}
	}
	if part.Slot != "" && part.Slot != "background" {
		if img, err := png.Decode(bytes.NewReader(raw)); err == nil {
			if o, ok := img.(interface{ Opaque() bool }); ok && o.Opaque() {
				warns = append(warns, "This image has no transparent pixels, so it covers everything beneath it. A frame layer is usually cut out")
			}
		}
	}
	return warns
}

// FontRequest is a font file to add to a draft. Assign lists the roles to draw with
// it
type FontRequest struct {
	ID     string   `json:"id"`
	Path   string   `json:"path"`
	Assign []string `json:"assign"`
}

var fontIDChars = regexp.MustCompile(`[^a-z0-9]+`)

// ImportFont copies a font into the draft's font folder, records it in template.json
// with its hash and the names it carries, and gives the roles in Assign to it
func (s *Service) ImportFont(req FontRequest) (ImportResult, error) {
	ext := strings.ToLower(path.Ext(req.Path))
	if ext != ".ttf" && ext != ".otf" {
		return ImportResult{}, apierr.New(apierr.BadRequest, "A font is a .ttf or .otf file")
	}
	raw, err := readFileLimited(req.Path, fonts.MaxFontBytes, "font")
	if err != nil {
		return ImportResult{}, err
	}
	family, style, err := fonts.Describe(raw)
	if err != nil {
		return ImportResult{}, apierr.Wrap(apierr.BadRequest, "That is not a font the engine can read", err)
	}
	fsys, err := s.store.FS(req.ID)
	if err != nil {
		return ImportResult{}, err
	}
	src, err := readSource(fsys)
	if err != nil {
		return ImportResult{}, apierr.Wrap(apierr.BadRequest, "The template's own file cannot be read, so fix it in the JSON first", err)
	}
	id := fontIDChars.ReplaceAllString(strings.ToLower(family+" "+style), "-")
	id = strings.Trim(id, "-")
	if id == "" {
		id = "font"
	}
	base := id
	for n := 2; ; n++ {
		if _, taken := src.Fonts[id]; !taken {
			break
		}
		id = base + "-" + strconv.Itoa(n)
	}
	file := "font/" + id + ext
	sum := sha256Hex(raw)

	value, _ := json.Marshal(manifest.FontFile{File: file, Family: family, Style: style, SHA256: sum})
	ops := []draftedit.Op{
		{Op: draftedit.OpPut, File: file, Stash: s.held.Put(raw)},
		{Op: draftedit.OpSet, Kind: draftedit.KindTemplate, Path: []string{"fonts", id}, Value: value},
	}
	// Fonts and roles need a newer contract than some templates are written to
	if !contractAtLeast(src.Contract, 1, 1) {
		ops = append(ops, draftedit.Op{Op: draftedit.OpSet, Kind: draftedit.KindTemplate, Path: []string{"contract"}, Value: json.RawMessage(strconv.Quote(manifest.Contract))})
	}
	for _, role := range req.Assign {
		assign, _ := json.Marshal(manifest.FontRole{Font: id})
		ops = append(ops, draftedit.Op{Op: draftedit.OpSet, Kind: draftedit.KindTemplate, Path: []string{"roles", role}, Value: assign})
	}
	res, err := s.Apply(ApplyRequest{ID: req.ID, Ops: ops})
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{ApplyResult: res, ID: id, Warnings: []string{}}, nil
}

// contractAtLeast reports whether a contract version such as "1.1" is at least
// major.minor
func contractAtLeast(contract string, major, minor int) bool {
	a, b, ok := strings.Cut(contract, ".")
	ma, err1 := strconv.Atoi(a)
	mi, err2 := strconv.Atoi(b)
	if !ok || err1 != nil || err2 != nil {
		return false
	}
	return ma > major || ma == major && mi >= minor
}

// MarksRequest is what the editor remembers about a draft's elements
type MarksRequest struct {
	ID     string   `json:"id"`
	Locked []string `json:"locked"`
	SnapTo []string `json:"snapTo"`
}

const maxMarks = 2000

// SetMarks keeps which elements are locked and which ones a drag snaps to
func (s *Service) SetMarks(req MarksRequest) error {
	if len(req.Locked) > maxMarks || len(req.SnapTo) > maxMarks {
		return apierr.New(apierr.BadRequest, "There are too many marks to keep")
	}
	meta, err := s.store.Meta(req.ID)
	if err != nil {
		return err
	}
	meta.Locked, meta.SnapTo = cleanMarks(req.Locked), cleanMarks(req.SnapTo)
	return s.store.SaveMeta(req.ID, meta)
}

// cleanMarks drops repeats and empty names, keeping the order
func cleanMarks(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range in {
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
