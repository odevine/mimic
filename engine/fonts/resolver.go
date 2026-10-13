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

// Font returns the sized-on-demand font for role. It never fails: a role
// nothing defines yields a Sizer on the basicfont fallback
func (r *Resolver) Font(role Role) *Sizer {
	f, _ := r.get(role)
	if f == nil {
		return &Sizer{fallback: true}
	}
	return &Sizer{font: f}
}

// Inspect reports how role resolved
func (r *Resolver) Inspect(role Role) Resolution {
	_, res := r.get(role)
	return res
}

func (r *Resolver) get(role Role) (*opentype.Font, Resolution) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.done[role]; ok {
		return v.font, v.res
	}
	f, res := r.resolve(role)
	r.done[role] = resolved{font: f, res: res}
	return f, res
}

func (r *Resolver) resolve(role Role) (*opentype.Font, Resolution) {
	res := Resolution{Role: role, Source: SourceFallback}
	own, ok := r.cat.own(role)
	if !ok {
		return nil, res
	}
	if name := r.overrides[string(role)]; name != "" {
		res.Override = name
		res.Source = SourceMissingOverride
		if pf, found := r.pool.Lookup(name); found {
			f, err := fontFromFile(pf.Path)
			if err == nil {
				res.Source, res.Path = SourceOverride, pf.Path
				res.Family, res.Style = pf.Family, pf.Style
				return f, res
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
	return own.font, res
}
