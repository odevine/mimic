// Package fonts owns font sourcing for the engine. It resolves a role (title,
// body, mana, symbols) to a font.Face through a chain: a font the end user
// dropped into an override directory, then a default compiled into the binary,
// then basicfont.Face7x13 as a last resort.
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
	// Mana is the mana-cost symbols
	Mana
	// Symbols is other glyphs such as tap symbols in rules text
	Symbols
)

//go:embed embedded
var embedded embed.FS

// embeddedPath maps a role to its compiled-in default. A role without an entry
// resolves to the basicfont fallback
var embeddedPath = map[Role]string{
	Title:      "embedded/title/BigShoulders-Bold.ttf",
	Body:       "embedded/body/Merriweather-Regular.ttf",
	BodyItalic: "embedded/body/Merriweather-Italic.ttf",
	Mana:       "embedded/mana/mana.ttf",
	Symbols:    "embedded/symbols/NDPMTG.ttf",
}

// userStems maps a role to the filename fragments an override file is matched
// by, case-insensitively. A dropped "Beleren.ttf" fills the Title role
var userStems = map[Role][]string{
	Title:      {"beleren"},
	Body:       {"plantin", "mplantin"},
	BodyItalic: {"plantin", "mplantin"},
	Mana:       {"mana"},
	Symbols:    {"ndpmtg", "proxyglyph", "glyph"},
}

var (
	cacheMu   sync.Mutex
	fontCache = map[string]*opentype.Font{}
)

// Resolve returns a face for role at the given point size. It tries an override
// file in userDir first, then the embedded default, then basicfont.Face7x13.
// The bool is true only when the basicfont fallback was used, so a caller can
// decide whether to normalize text for that limited face. An empty userDir
// skips the override step
func Resolve(role Role, userDir string, size float64) (font.Face, bool, error) {
	if userDir != "" {
		if path := findUserFont(role, userDir); path != "" {
			if face, err := faceFromFile(path, size); err == nil {
				return face, false, nil
			}
		}
	}
	if path, ok := embeddedPath[role]; ok {
		if face, err := faceFromEmbedded(path, size); err == nil {
			return face, false, nil
		}
	}
	return basicfont.Face7x13, true, nil
}

func faceFromFile(path string, size float64) (font.Face, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parse("file:"+path, raw)
	if err != nil {
		return nil, err
	}
	return newFace(f, size)
}

func faceFromEmbedded(path string, size float64) (font.Face, error) {
	raw, err := embedded.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parse("embed:"+path, raw)
	if err != nil {
		return nil, err
	}
	return newFace(f, size)
}

// parse returns the parsed font for a cache key, parsing and caching on a miss.
// A parsed font is reused across renders, while each Resolve builds its own face
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
