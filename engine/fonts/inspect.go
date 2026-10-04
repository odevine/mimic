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
	// Folder is the role's subfolder under a font override directory
	Folder string
	// Borrows is the manifest name of the role whose default this one uses
	// when it bundles none, or "" when it has a default of its own
	Borrows string
}

// roleOrder lists every role in the order a caller should present them
var roleOrder = []Role{Title, Body, BodyItalic, Mana, SmallCaps, Info}

// Roles returns every role with an override folder, in presentation order
func Roles() []RoleInfo {
	out := make([]RoleInfo, 0, len(roleOrder))
	for _, r := range roleOrder {
		info := RoleInfo{Role: r, Name: r.String(), Folder: roleDir[r]}
		if fb, ok := fallbackRole[r]; ok {
			info.Borrows = fb.String()
		}
		out = append(out, info)
	}
	return out
}

// String returns the role's manifest name, or "" for a role with none
func (r Role) String() string {
	for name, role := range roleNames {
		if role == r {
			return name
		}
	}
	return ""
}

// Source is where a role's resolved font came from
type Source string

const (
	// SourceUser is a font from the role's subfolder of the override directory
	SourceUser Source = "user"
	// SourceDefault is the role's own embedded default
	SourceDefault Source = "default"
	// SourceBorrowed is the embedded default of a fallback role, used by a role
	// that bundles none of its own
	SourceBorrowed Source = "borrowed"
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
