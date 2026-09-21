// Package fonts owns font sourcing for the engine. It resolves a role (title,
// body, mana) to a font.Face through a chain: a font the end user dropped into
// an override directory, then a default compiled into the binary, then
// basicfont.Face7x13 as a last resort.
//
// The engine bundles no facsimile of the real Magic fonts. Titles fall back to
// Big Shoulders and body text to Merriweather, both under the SIL Open Font
// License, and a user who has obtained Beleren or Plantin supplies them through
// the override directory.
package fonts

import (
	"embed"
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
)

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

// userStems maps a role to the filename fragments an override file is matched
// by, case-insensitively. A dropped "Beleren.ttf" fills the Title role
var userStems = map[Role][]string{
	Title:      {"beleren"},
	Body:       {"plantin", "mplantin"},
	BodyItalic: {"plantin", "mplantin"},
	Mana:       {"mana"},
}

var (
	cacheMu   sync.Mutex
	fontCache = map[string]*opentype.Font{}
)

// Sizer holds a role's resolved font so faces can be built at any point size
// without re-reading or re-parsing the file. A pass that tries several sizes
// for one text box uses one Sizer rather than resolving once per size
type Sizer struct {
	font     *opentype.Font // nil means the basicfont fallback
	fallback bool
}

// ResolveFont picks the font for role the way Resolve does, an override file in
// userDir first, then the embedded default, then the basicfont fallback, but
// leaves sizing to Face. An empty userDir skips the override step. It never
// fails: an unresolvable role yields a Sizer whose Face returns the fallback
func ResolveFont(role Role, userDir string) *Sizer {
	if userDir != "" {
		if path := findUserFont(role, userDir); path != "" {
			if f, err := fontFromFile(path); err == nil {
				return &Sizer{font: f}
			}
		}
	}
	if path, ok := embeddedPath[role]; ok {
		if f, err := fontFromEmbedded(path); err == nil {
			return &Sizer{font: f}
		}
	}
	return &Sizer{fallback: true}
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

// Resolve returns a face for role at the given point size. It tries an override
// file in userDir first, then the embedded default, then basicfont.Face7x13.
// The bool is true only when the basicfont fallback was used, so a caller can
// decide whether to normalize text for that limited face. An empty userDir
// skips the override step
func Resolve(role Role, userDir string, size float64) (font.Face, bool, error) {
	s := ResolveFont(role, userDir)
	face, err := s.Face(size)
	return face, s.fallback, err
}

func fontFromFile(path string) (*opentype.Font, error) {
	key := "file:" + path
	if f := cachedFont(key); f != nil {
		return f, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(key, raw)
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

// findUserFont returns the first override file in dir matching role, or "" when
// none matches. Body and BodyItalic share stems and are told apart by whether
// the filename contains "italic"
func findUserFont(role Role, dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	wantItalic := role == BodyItalic
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if ext := filepath.Ext(name); ext != ".ttf" && ext != ".otf" {
			continue
		}
		if role == Body || role == BodyItalic {
			if strings.Contains(name, "italic") != wantItalic {
				continue
			}
		}
		for _, stem := range userStems[role] {
			if strings.Contains(name, stem) {
				return filepath.Join(dir, e.Name())
			}
		}
	}
	return ""
}
