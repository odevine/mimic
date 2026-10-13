package fonts

import (
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// RoleInfo describes one role for a caller that lists them, such as a settings
// panel showing which font each role draws with
type RoleInfo struct {
	Role Role
	// Name is the manifest name a text box uses for the role, as ParseRole reads it
	Name string
	// Label is how a settings panel names the role
	Label string
	// Folder is the role's subfolder under a font override directory
	Folder string
	// Builtin is the id of the built-in font the role draws with by default
	Builtin string
}

// roleOrder lists every role in the order a caller should present them
var roleOrder = []Role{Title, Body, BodyItalic, Mana, SmallCaps, Info}

// Roles returns every standard role, in presentation order
func Roles() []RoleInfo {
	out := make([]RoleInfo, 0, len(roleOrder))
	for _, r := range roleOrder {
		def := standardRoles[r]
		out = append(out, RoleInfo{Role: r, Name: r.String(), Label: def.label, Folder: roleDir[r], Builtin: def.builtin})
	}
	return out
}

// String returns the role's manifest name
func (r Role) String() string { return string(r) }

// Builtins returns every built-in font in presentation order
func Builtins() []Builtin { return append([]Builtin(nil), builtins...) }

// Source is where a role's resolved font came from
type Source string

const (
	// SourceUser is a font from the role's subfolder of the override directory
	SourceUser Source = "user"
	// SourceDefault is the role's built-in font
	SourceDefault Source = "default"
	// SourceFallback is basicfont.Face7x13, used when nothing else resolved
	SourceFallback Source = "fallback"
)

// Resolution reports how a role resolved, the font itself plus what was passed
// over on the way, so a caller can explain a silent fallback
type Resolution struct {
	Role   Role
	Source Source
	// Path is the user file for SourceUser and the embedded path for a default
	Path string
	// Family and Style come from the font's name table
	Family, Style string
	// Rejected is a user file that failed to load, and Err why. A rejected
	// file leaves the role on its default
	Rejected string
	Err      error
	// Ignored lists further font files in the role's folder, which are never
	// used since only the first one counts
	Ignored []string
}

// Inspect resolves role exactly as ResolveFont does and reports the outcome
func Inspect(role Role, userDir string) Resolution {
	_, res := resolve(role, userDir)
	return res
}

// names reads a font's family and style from its name table, preferring the
// typographic names, since the legacy ones fold weight and optical size into
// the family. Either is empty when the table lacks it
func names(f *opentype.Font) (family, style string) {
	var buf sfnt.Buffer
	name := func(ids ...sfnt.NameID) string {
		for _, id := range ids {
			if s, err := f.Name(&buf, id); err == nil && s != "" {
				return s
			}
		}
		return ""
	}
	family = name(sfnt.NameIDTypographicFamily, sfnt.NameIDFamily)
	style = name(sfnt.NameIDTypographicSubfamily, sfnt.NameIDSubfamily)
	return family, style
}

// Validate reports whether raw parses as a font the engine can draw with, so a
// caller can reject a file before storing it
func Validate(raw []byte) error {
	_, err := opentype.Parse(raw)
	return err
}
