package fonts

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// maxPoolFiles bounds how many font files a pool reads, since the folder it
// scans is the user's and may hold a whole system's worth
const maxPoolFiles = 2000

// PoolFont is one font file in a pool
type PoolFont struct {
	// Name is the font's family and style joined by a space, such as "Plantin
	// MT Pro Italic", which is how a role is assigned the font
	Name          string
	Family, Style string
	Path          string
}

// PoolReject is a font file a pool could not use and why
type PoolReject struct {
	Path string
	Err  error
}

// Pool is the set of font files a user has made available, found by their
// family and style names, so any font can be assigned to any role. A pool is
// read once when it is made and does not change after, so a caller makes a new
// one when the folder changes. A nil Pool is an empty one
type Pool struct {
	dir      string
	fonts    []PoolFont
	byName   map[string]int
	dups     []string
	rejected []PoolReject
}

// NewPool indexes every .ttf and .otf under dir, subfolders included. An empty
// or missing dir gives an empty pool. When two files carry the same family and
// style, the first by path wins and the other is listed in Duplicates
func NewPool(dir string) *Pool {
	p := &Pool{dir: dir, byName: map[string]int{}}
	if dir == "" {
		return p
	}
	seen := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !isFontFile(d.Name()) || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if seen++; seen > maxPoolFiles {
			return fs.SkipAll
		}
		p.add(path)
		return nil
	})
	return p
}

// isFontFile reports whether a file name has a font extension the engine reads
func isFontFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ttf", ".otf":
		return true
	}
	return false
}

func (p *Pool) add(path string) {
	f, err := fontFromFile(path)
	if err != nil {
		p.rejected = append(p.rejected, PoolReject{Path: path, Err: err})
		return
	}
	family, style := names(f)
	if family == "" {
		p.rejected = append(p.rejected, PoolReject{Path: path, Err: fmt.Errorf("the font has no family name")})
		return
	}
	if style == "" {
		style = "Regular"
	}
	name := family + " " + style
	key := normalizeName(name)
	if _, taken := p.byName[key]; taken {
		p.dups = append(p.dups, path)
		return
	}
	p.byName[key] = len(p.fonts)
	p.fonts = append(p.fonts, PoolFont{Name: name, Family: family, Style: style, Path: path})
}

// normalizeName folds case and runs of space, so "plantin  mt pro italic"
// finds "Plantin MT Pro Italic"
func normalizeName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// Dir is the folder the pool was read from
func (p *Pool) Dir() string {
	if p == nil {
		return ""
	}
	return p.dir
}

// Fonts lists every usable font in the pool by path
func (p *Pool) Fonts() []PoolFont {
	if p == nil {
		return nil
	}
	return append([]PoolFont(nil), p.fonts...)
}

// Lookup finds a font by its name, such as "Plantin MT Pro Italic", ignoring
// case
func (p *Pool) Lookup(name string) (PoolFont, bool) {
	if p == nil {
		return PoolFont{}, false
	}
	i, ok := p.byName[normalizeName(name)]
	if !ok {
		return PoolFont{}, false
	}
	return p.fonts[i], true
}

// Duplicates lists the files left out because an earlier one has the same name
func (p *Pool) Duplicates() []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.dups...)
}

// Rejected lists the font files that could not be read as a font
func (p *Pool) Rejected() []PoolReject {
	if p == nil {
		return nil
	}
	return append([]PoolReject(nil), p.rejected...)
}
