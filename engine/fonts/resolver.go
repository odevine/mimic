package fonts

import (
	"sync"

	"golang.org/x/image/font/opentype"
)

// Source is where a role's resolved font came from
type Source string

const (
	// SourceOverride is the font the user assigned to the role, found in the pool
	SourceOverride Source = "override"
	// SourceTemplate is the role's own font, built-in or bundled with the
	// template, because the user assigned none
	SourceTemplate Source = "template"
	// SourceMissingOverride is the role's own font, used because the font the
	// user assigned is not in the pool
	SourceMissingOverride Source = "template-missing-override"
	// SourceFallback is basicfont.Face7x13, used for a role nothing defines
	SourceFallback Source = "fallback"
)

// Resolution reports how a role resolved, so a caller can show what a render
// draws with and explain a fallback
type Resolution struct {
	Role   Role
	Source Source
	// Path is the pool file for SourceOverride and the embedded path for a
	// built-in role font, and empty for a bundled one
	Path string
	// Family and Style come from the font in use
	Family, Style string
	// Override is the name the user assigned to the role, set whether or not
	// it was found
	Override string
	// Err is why an assigned font that is in the pool could not be read
	Err error
}

// Resolver picks the font each role draws with for one render: the font the
// user assigned from the pool, else the role's own. A role is resolved once and
// remembered, so a card's many text boxes share it. It is safe for concurrent
// use
type Resolver struct {
	cat       *Catalog
	pool      *Pool
	overrides map[string]string

	mu   sync.Mutex
	done map[Role]resolved
}

type resolved struct {
	font *opentype.Font // nil means the basicfont fallback
	more []*opentype.Font
	res  Resolution
}

// NewResolver makes a resolver over a template's catalog, a pool of user fonts,
// and the font name the user assigned to each role by its name. Any of them may
// be nil
func NewResolver(cat *Catalog, pool *Pool, overrides map[string]string) *Resolver {
	cp := make(map[string]string, len(overrides))
	for k, v := range overrides {
		cp[k] = v
	}
	return &Resolver{cat: cat, pool: pool, overrides: cp, done: map[Role]resolved{}}
}

// Role resolves the name a text box uses to a role, and reports false for one
// neither the template nor the engine defines
func (r *Resolver) Role(name string) (Role, bool) {
	role := Role(name)
	return role, r.cat.Knows(role)
}

// Font returns the sized-on-demand font for role. A rune its font lacks is
// drawn from the next of the role's own font and the built-in body font. It
// never fails: a role nothing defines yields a Sizer on the basicfont fallback
func (r *Resolver) Font(role Role) *Sizer {
	v := r.get(role)
	if v.font == nil {
		return &Sizer{fallback: true}
	}
	return &Sizer{font: v.font, more: v.more}
}

// Inspect reports how role resolved
func (r *Resolver) Inspect(role Role) Resolution { return r.get(role).res }

func (r *Resolver) get(role Role) resolved {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.done[role]; ok {
		return v
	}
	v := r.resolve(role)
	r.done[role] = v
	return v
}

// fallbacks lists the fonts a rune is drawn from when the first has no glyph for
// it: the role's own font, when the first is one the user assigned, and the
// built-in body font, which covers the Latin letters the card fonts leave out.
// A font already in the chain is not listed again
func fallbacks(first, own *opentype.Font) []*opentype.Font {
	chain := []*opentype.Font{first}
	add := func(f *opentype.Font) {
		for _, have := range chain {
			if have == f {
				return
			}
		}
		chain = append(chain, f)
	}
	add(own)
	if body, err := fontFromEmbedded(builtinFile[standardRoles[Body].builtin]); err == nil {
		add(body)
	}
	return chain[1:]
}

func (r *Resolver) resolve(role Role) resolved {
	res := Resolution{Role: role, Source: SourceFallback}
	own, ok := r.cat.own(role)
	if !ok {
		return resolved{res: res}
	}
	if name := r.overrides[string(role)]; name != "" {
		res.Override = name
		res.Source = SourceMissingOverride
		if pf, found := r.pool.Lookup(name); found {
			f, err := fontFromFile(pf.Path)
			if err == nil {
				res.Source, res.Path = SourceOverride, pf.Path
				res.Family, res.Style = pf.Family, pf.Style
				return resolved{font: f, more: fallbacks(f, own.font), res: res}
			}
			res.Err = err
		}
	} else {
		res.Source = SourceTemplate
	}
	res.Family, res.Style = own.family, own.style
	if own.builtin != "" {
		res.Path = builtinFile[own.builtin]
	}
	return resolved{font: own.font, more: fallbacks(own.font, own.font), res: res}
}
