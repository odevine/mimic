// Package fonts owns font sourcing for the engine. It resolves a role (title,
// body, mana) to a font.Face through a chain: a font the end user dropped into
// that role's subfolder of an override directory, then a default compiled into
// the binary, then a fallback role's default for a role that bundles none, then
// basicfont.Face7x13 as a last resort.
//
// The engine bundles no facsimile of the real Magic fonts. Titles fall back to
// Big Shoulders and body text to Merriweather, both under the SIL Open Font
// License, and a user who has obtained Beleren or Plantin supplies them through
// the override directory.
package fonts

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

// Role is the part a font plays on a card. A template maps each text box to one
type Role int

const (
	// Title is the card name, type line, and power/toughness, the Beleren role
	Title Role = iota
	// Body is rules text, the Plantin role
	Body
	// BodyItalic is flavor text, the Plantin italic role
	BodyItalic
	// Mana is every card symbol: mana costs, tap and untap, loyalty, card
	// types, and watermarks. The Mana font covers all of them in one face
	Mana
	// SmallCaps is the artist credit's small-caps face. It bundles no default
	// and borrows the Title default when the user supplies no font
	SmallCaps
	// Info is the sans face for the collector, set, and copyright lines. It
	// bundles no default and borrows the Body default
	Info
)

// roleNames maps a manifest's Font string to the Role it names, so a text box
// declares its font role as data rather than a template mapping box names to
// roles in Go
var roleNames = map[string]Role{
	"title":       Title,
	"body":        Body,
	"body-italic": BodyItalic,
	"mana":        Mana,
	"small-caps":  SmallCaps,
	"info":        Info,
}

// ParseRole resolves a manifest's Font string to the Role it names, reporting
// false for an empty or unrecognized one, which a caller treats as the body
// role
func ParseRole(name string) (Role, bool) {
	role, ok := roleNames[name]
	return role, ok
}

//go:embed embedded
var embedded embed.FS

// embeddedPath maps a role to its compiled-in default. A role without an entry
// resolves to the basicfont fallback. The mana face is Mana 1.18.0, whose
// glyphs live in the private use area rather than at ASCII letters
var embeddedPath = map[Role]string{
	Title:      "embedded/title/BigShoulders-Bold.ttf",
	Body:       "embedded/body/Merriweather-Regular.ttf",
	BodyItalic: "embedded/body/Merriweather-Italic.ttf",
	Mana:       "embedded/mana/mana.ttf",
}

// embeddedNames is the family and style each embedded default presents as. The
// files are static instances cut from variable fonts, and their name tables
// still carry the source instance's names, such as Big Shoulders Thin for the
// bold cut
var embeddedNames = map[string][2]string{
	"embedded/title/BigShoulders-Bold.ttf":   {"Big Shoulders", "Bold"},
	"embedded/body/Merriweather-Regular.ttf": {"Merriweather", "Regular"},
	"embedded/body/Merriweather-Italic.ttf":  {"Merriweather", "Italic"},
	"embedded/mana/mana.ttf":                 {"Mana", "Regular"},
}

// roleDir maps a role to its subfolder under the user font directory. A user
// drops a font into the folder for its role, so no file needs a matching name.
// A role without an entry has no override folder and always uses its default
var roleDir = map[Role]string{
	Title:      "title",
	Body:       "body",
	BodyItalic: "body-italic",
	Mana:       "mana",
	SmallCaps:  "type",
	Info:       "info",
}

// fallbackRole borrows another role's default for a role that bundles no font
// of its own, so a user who drops in no override still gets a fitting face
// rather than the bitmap fallback
var fallbackRole = map[Role]Role{
	SmallCaps: Title,
	Info:      Body,
}

var (
	cacheMu   sync.Mutex
	fontCache = map[string]*opentype.Font{}
	// fileKeys maps a user font's path to its current cache key, so a replaced
	// file's stale parse can be evicted
	fileKeys = map[string]string{}
)

// Sizer holds a role's resolved font so faces can be built at any point size
// without re-reading or re-parsing the file. A pass that tries several sizes
// for one text box uses one Sizer rather than resolving once per size
type Sizer struct {
	font     *opentype.Font // nil means the basicfont fallback
	fallback bool
}

// ResolveFont picks the font for role the way Resolve does, the role's subfolder
// of userDir first, then its embedded default, then the default of a fallback
// role, then the basicfont fallback, but leaves sizing to Face. An empty userDir
// skips the override step. It never fails: an unresolvable role yields a Sizer
// whose Face returns the fallback
func ResolveFont(role Role, userDir string) *Sizer {
	f, _ := resolve(role, userDir)
	if f == nil {
		return &Sizer{fallback: true}
	}
	return &Sizer{font: f}
}

// resolve walks the resolution chain for role and returns the parsed font, nil
// for the basicfont fallback, beside a Resolution describing where it came from
func resolve(role Role, userDir string) (*opentype.Font, Resolution) {
	res := Resolution{Role: role, Source: SourceFallback}
	if userDir != "" {
		files := userFonts(role, userDir)
		if len(files) > 0 {
			res.Ignored = files[1:]
			f, err := fontFromFile(files[0])
			if err == nil {
				res.Source, res.Path = SourceUser, files[0]
				res.Family, res.Style = names(f)
				return f, res
			}
			res.Rejected, res.Err = files[0], err
		}
	}
	for r := role; ; {
		if path, ok := embeddedPath[r]; ok {
			if f, err := fontFromEmbedded(path); err == nil {
				res.Source, res.Path = SourceDefault, path
				if r != role {
					res.Source = SourceBorrowed
				}
				res.Family, res.Style = embeddedNames[path][0], embeddedNames[path][1]
				return f, res
			}
		}
		next, ok := fallbackRole[r]
		if !ok {
			break
		}
		r = next
	}
	return nil, res
}

// Fallback reports whether Face returns basicfont.Face7x13, so a caller can
// decide whether to normalize text for that limited face
func (s *Sizer) Fallback() bool { return s.fallback }

// Face returns a face for the resolved font at size, or basicfont.Face7x13
// when no font resolved. The fallback is a fixed size and ignores size
func (s *Sizer) Face(size float64) (font.Face, error) {
	if s.font == nil {
		return basicfont.Face7x13, nil
	}
	return newFace(s.font, size)
}

// Resolve returns a face for role at the given point size. It tries the role's
// subfolder of userDir first, then the embedded default, then basicfont.Face7x13.
// The bool is true only when the basicfont fallback was used, so a caller can
// decide whether to normalize text for that limited face. An empty userDir
// skips the override step
func Resolve(role Role, userDir string, size float64) (font.Face, bool, error) {
	s := ResolveFont(role, userDir)
	face, err := s.Face(size)
	return face, s.fallback, err
}

// fontFromFile parses a user font, caching it under its path, size, and
// modification time so a file replaced under the same name is read again. The
// entry for an older version of the same path is dropped when a newer one lands
func fontFromFile(path string) (*opentype.Font, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("file:%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())
	if f := cachedFont(key); f != nil {
		return f, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parse(key, raw)
	if err != nil {
		return nil, err
	}
	cacheMu.Lock()
	if old, ok := fileKeys[path]; ok && old != key {
		delete(fontCache, old)
	}
	fileKeys[path] = key
	cacheMu.Unlock()
	return f, nil
}

func fontFromEmbedded(path string) (*opentype.Font, error) {
	key := "embed:" + path
	if f := cachedFont(key); f != nil {
		return f, nil
	}
	raw, err := embedded.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(key, raw)
}

// cachedFont returns the parsed font for key, or nil when none is cached, so a
// caller can skip reading the file on a hit
func cachedFont(key string) *opentype.Font {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return fontCache[key]
}

// parse returns the parsed font for a cache key, parsing and caching on a miss.
// A parsed font is reused across renders, while each Face builds its own sized
// face from it
func parse(key string, raw []byte) (*opentype.Font, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if f, ok := fontCache[key]; ok {
		return f, nil
	}
	f, err := opentype.Parse(raw)
	if err != nil {
		return nil, err
	}
	fontCache[key] = f
	return f, nil
}

func newFace(f *opentype.Font, size float64) (font.Face, error) {
	if size <= 0 {
		size = 16
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

// findUserFont returns the first .ttf or .otf in role's subfolder of dir, or ""
// when the folder is absent or holds no font. One folder per role means the
// dropped file keeps its own name
func findUserFont(role Role, dir string) string {
	if files := userFonts(role, dir); len(files) > 0 {
		return files[0]
	}
	return ""
}

// userFonts lists every .ttf or .otf in role's subfolder of dir in directory
// order. The first is the one used, and the rest are reported as ignored
func userFonts(role Role, dir string) []string {
	sub, ok := roleDir[role]
	if !ok {
		return nil
	}
	folder := filepath.Join(dir, sub)
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := strings.ToLower(filepath.Ext(e.Name())); ext == ".ttf" || ext == ".otf" {
			out = append(out, filepath.Join(folder, e.Name()))
		}
	}
	return out
}
