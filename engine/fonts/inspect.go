package fonts

import (
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// RoleInfo describes one role for a caller that lists them, such as a settings
// panel showing which font each role draws with
type RoleInfo struct {
	Role Role
	// Name is the name a text box uses for the role
	Name string
	// Label is how a settings panel names the role
	Label string
	// Family and Style name the role's own font, before any the user assigns
	Family, Style string
	// Builtin is the id of the engine font the role draws with, or "" when the
	// template bundles the font
	Builtin string
	// Bundled is true when the template ships the role's font
	Bundled bool
}

// Roles returns every standard role with its built-in font, in presentation
// order. A template adds its own through its Catalog
func Roles() []RoleInfo { return (*Catalog)(nil).Roles() }

// String returns the role's name
func (r Role) String() string { return string(r) }

// Builtins returns every built-in font in presentation order
func Builtins() []Builtin { return append([]Builtin(nil), builtins...) }

// IsBuiltin reports whether id names a font compiled into the engine
func IsBuiltin(id string) bool {
	_, ok := builtinFile[id]
	return ok
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

// Describe parses raw as a font and returns its family and style, so a tool
// that packs a font can record what it is called
func Describe(raw []byte) (family, style string, err error) {
	f, err := opentype.Parse(raw)
	if err != nil {
		return "", "", err
	}
	family, style = names(f)
	return family, style, nil
}
