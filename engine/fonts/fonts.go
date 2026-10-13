// Package fonts owns font sourcing for the engine. A role (title, body, mana,
// or one a template defines) resolves to a font.Face through a chain: a font
// the user picked by name from a pool of font files, then the role's own font,
// which is a built-in one compiled into the binary or one the template bundles,
// then basicfont.Face7x13 as a last resort.
//
// The built-in fonts are reproductions of the fonts printed on Magic cards, so
// a render looks like a real card with nothing extra installed. A user who
// prefers another face adds its file to the pool and assigns it to a role.
package fonts

import (
	"embed"
	"fmt"
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

// Role is the part a font plays on a card. A text box names its role in the
// manifest, and the role is a plain string so a role is the same thing in a
// manifest, a settings file, and the code
type Role string

const (
	// Title is the card name, type line, and power/toughness
	Title Role = "title"
	// Body is rules text
	Body Role = "body"
	// BodyItalic is flavor text
	BodyItalic Role = "body-italic"
	// Mana is every card symbol: mana costs, tap and untap, loyalty, card
	// types, and watermarks. The Mana font covers all of them in one face
	Mana Role = "mana"
	// SmallCaps is the artist credit's small-caps face
	SmallCaps Role = "small-caps"
	// Info is the sans face for the collector, set, and copyright lines
	Info Role = "info"
)

// standardRole is what the engine knows about one of the standard roles
type standardRole struct {
	// label is how a settings panel names the role
	label string
	// builtin is the id of the built-in font the role draws with by default
	builtin string
}

// standardRoles lists the roles every template has, each with a label and the
// built-in font it defaults to
var standardRoles = map[Role]standardRole{
	Title:      {label: "Card name, type line and power/toughness", builtin: "mimic-m15-title"},
	Body:       {label: "Rules text", builtin: "mimic-m15-body"},
	BodyItalic: {label: "Flavor text", builtin: "mimic-m15-body-italic"},
	Mana:       {label: "Mana and other card symbols", builtin: "mimic-m15-mana"},
	SmallCaps:  {label: "Artist credit", builtin: "mimic-m15-small-caps"},
	Info:       {label: "Collector number and set code", builtin: "mimic-m15-info"},
}

// ParseRole resolves a manifest's Font string to the standard role it names,
// reporting false for an empty or unrecognized one, which a caller treats as
// the body role
func ParseRole(name string) (Role, bool) {
	role := Role(name)
	_, ok := standardRoles[role]
	return role, ok
}

//go:embed embedded
var embedded embed.FS

// Builtin describes one font compiled into the engine. The id is what a role
// names and is keyed by the role the font fills, not by its file or family, so
// the font behind an id can change without a template that names it breaking.
// An id is never renamed or reassigned to another role
type Builtin struct {
	ID string
	// File is the font's path inside the embedded tree
	File string
}

// builtins lists every built-in font in presentation order
var builtins = []Builtin{
	{ID: "mimic-m15-title", File: "embedded/m15/BelerenProxy-Bold.ttf"},
	{ID: "mimic-m15-body", File: "embedded/m15/plantin-body.ttf"},
	{ID: "mimic-m15-body-italic", File: "embedded/m15/plantin-italic.ttf"},
	{ID: "mimic-m15-small-caps", File: "embedded/m15/Beleren Small Caps.ttf"},
	{ID: "mimic-m15-info", File: "embedded/m15/Gotham-Medium.otf"},
	{ID: "mimic-m15-mana", File: "embedded/mana/mana.ttf"},
}

// builtinFile maps a built-in id to its embedded path
var builtinFile = func() map[string]string {
	m := make(map[string]string, len(builtins))
	for _, b := range builtins {
		m[b.ID] = b.File
	}
	return m
}()

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

// Default returns the built-in font of a standard role with no template or
// override in play, or a Sizer on the basicfont fallback for a role the engine
// does not know
func Default(role Role) *Sizer { return NewResolver(nil, nil, nil).Font(role) }

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
