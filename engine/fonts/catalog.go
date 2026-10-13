package fonts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"golang.org/x/image/font/opentype"
)

// MaxFontBytes is the largest bundled font file that is read. A bundle can come
// from anyone, so what a render allocates from it is bounded where it is read
const MaxFontBytes = 16 << 20

// FontDef is a font file a template bundles
type FontDef struct {
	// File is the font's path in the bundle
	File string
	// Family and Style are what the font is called. Empty reads them from the
	// font's own name table
	Family, Style string
	// SHA256 is the hex hash of the file. Empty skips the check, and a hash
	// that does not match the file is an error
	SHA256 string
}

// RoleDef is a text role a template defines, or redefines when it names a
// standard role. Exactly one of Font and Builtin is set
type RoleDef struct {
	// Label is how a settings panel names the role. Empty is the role's name
	Label string
	// Font is a key of the template's fonts
	Font string
	// Builtin is the id of a font compiled into the engine
	Builtin string
}

// Catalog is what a template says about its fonts: the roles it defines and the
// fonts it bundles, read and parsed once when the template loads. A nil Catalog
// is a template that defines nothing, whose standard roles use their built-in
// fonts
type Catalog struct {
	roles map[Role]*catalogRole
	order []Role
}

// catalogRole is a role a template defined, with its font ready to draw
type catalogRole struct {
	label         string
	font          *opentype.Font
	family, style string
	builtin       string
	bundled       bool
}

// NewCatalog reads each bundled font through open, checks and parses it, and
// binds each role to its font. Every problem found is in the error
func NewCatalog(defs map[string]FontDef, roles map[string]RoleDef, open func(file string) (io.ReadCloser, error)) (*Catalog, error) {
	var errs []error
	bundled := map[string]*catalogRole{}
	for _, id := range sortedKeys(defs) {
		def := defs[id]
		f, err := readBundled(def, open)
		if err != nil {
			errs = append(errs, fmt.Errorf("font %q: %w", id, err))
			continue
		}
		family, style := names(f)
		if def.Family != "" {
			family = def.Family
		}
		if def.Style != "" {
			style = def.Style
		}
		bundled[id] = &catalogRole{font: f, family: family, style: style, bundled: true}
	}
	c := &Catalog{roles: map[Role]*catalogRole{}}
	for _, name := range sortedKeys(roles) {
		def := roles[name]
		r := &catalogRole{label: def.Label}
		switch {
		case def.Font != "" && def.Builtin != "":
			errs = append(errs, fmt.Errorf("role %q names both a font and a built-in font", name))
			continue
		case def.Font != "":
			b, ok := bundled[def.Font]
			if !ok {
				if _, declared := defs[def.Font]; !declared {
					errs = append(errs, fmt.Errorf("role %q uses the font %q, which the template does not bundle", name, def.Font))
				}
				continue
			}
			r.font, r.family, r.style, r.bundled = b.font, b.family, b.style, true
		case def.Builtin != "":
			file, ok := builtinFile[def.Builtin]
			if !ok {
				errs = append(errs, fmt.Errorf("role %q uses the built-in font %q, which this engine does not have", name, def.Builtin))
				continue
			}
			f, err := fontFromEmbedded(file)
			if err != nil {
				errs = append(errs, fmt.Errorf("role %q: %w", name, err))
				continue
			}
			r.font, r.builtin = f, def.Builtin
			r.family, r.style = names(f)
		default:
			errs = append(errs, fmt.Errorf("role %q names no font", name))
			continue
		}
		c.roles[Role(name)] = r
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	c.order = standardOrder()
	for _, name := range sortedKeys(roles) {
		if _, standard := standardRoles[Role(name)]; !standard {
			c.order = append(c.order, Role(name))
		}
	}
	return c, nil
}

// readBundled reads one bundled font, checks its hash and parses it. The parsed
// font is cached under the hash of its bytes, so a font that two templates
// bundle is parsed once
func readBundled(def FontDef, open func(string) (io.ReadCloser, error)) (*opentype.Font, error) {
	rc, err := open(def.File)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, MaxFontBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxFontBytes {
		return nil, fmt.Errorf("%s is over the limit of %d bytes", def.File, MaxFontBytes)
	}
	sum := sha256.Sum256(raw)
	got := hex.EncodeToString(sum[:])
	if def.SHA256 != "" && !strings.EqualFold(def.SHA256, got) {
		return nil, fmt.Errorf("%s does not match its sha256", def.File)
	}
	f, err := parse("sha:"+got, raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not a font: %w", def.File, err)
	}
	return f, nil
}

// own returns the font a role draws with when the user picks none, from the
// template's definition of the role or else the standard one. It reports false
// for a role neither names
func (c *Catalog) own(role Role) (*catalogRole, bool) {
	if c != nil {
		if r, ok := c.roles[role]; ok {
			return r, true
		}
	}
	def, ok := standardRoles[role]
	if !ok {
		return nil, false
	}
	file := builtinFile[def.builtin]
	f, err := fontFromEmbedded(file)
	if err != nil {
		return nil, false
	}
	family, style := names(f)
	return &catalogRole{label: def.label, font: f, family: family, style: style, builtin: def.builtin}, true
}

// Knows reports whether the template defines role or it is a standard one
func (c *Catalog) Knows(role Role) bool {
	_, ok := c.own(role)
	return ok
}

// Roles lists every role the template has, the standard ones first in their
// usual order and then the ones it defines, by name
func (c *Catalog) Roles() []RoleInfo {
	order := standardOrder()
	if c != nil {
		order = c.order
	}
	out := make([]RoleInfo, 0, len(order))
	for _, role := range order {
		r, ok := c.own(role)
		if !ok {
			continue
		}
		label := r.label
		if label == "" {
			label = string(role)
		}
		out = append(out, RoleInfo{
			Role: role, Name: string(role), Label: label,
			Family: r.family, Style: r.style, Builtin: r.builtin, Bundled: r.bundled,
		})
	}
	return out
}

func standardOrder() []Role {
	return []Role{Title, Body, BodyItalic, Mana, SmallCaps, Info}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
