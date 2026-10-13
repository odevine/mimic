// Package fontdir picks the one folder of fonts a user can assign to a role,
// manages the app's own fonts folder, the writable one the settings panel adds
// fonts to and removes them from, and keeps the engine's index of the folder
package fontdir

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/odevine/mimic/engine/fonts"
)

// Kind is which source the active directory came from
type Kind string

const (
	// KindFlag is a directory named by the -fonts flag
	KindFlag Kind = "flag"
	// KindCheckout is local-fonts in a repository checkout
	KindCheckout Kind = "checkout"
	// KindManaged is the app's own folder under the user config directory
	KindManaged Kind = "managed"
	// KindNone means there is no directory at all, so no font can be assigned
	KindNone Kind = "none"
)

// CheckoutCandidates are where a checkout's local-fonts folder sits relative
// to a repo-root run and a ui-subdir run
var CheckoutCandidates = []string{"local-fonts", filepath.Join("..", "local-fonts")}

// MaxFontBytes bounds an added font. Real card fonts are well under 1 MB
const MaxFontBytes = 20 << 20

// ErrReadOnly is returned when writing to a directory the app does not own
var ErrReadOnly = errors.New("the active fonts folder is not the app's own, so it is read-only")

// Dir is the active folder of fonts. Path is "" for KindNone
type Dir struct {
	Path string
	Kind Kind
}

// Writable reports whether the app may add and remove fonts in the directory
func (d Dir) Writable() bool { return d.Kind == KindManaged }

// Resolve picks the font directory: flag when set and present, then a checkout's
// local-fonts, then managed. A missing flag directory is logged and skipped.
// managed need not exist yet, since it is created on the first write, and ""
// means there is no config directory to put it in
func Resolve(flag, managed string) Dir {
	if flag != "" {
		if isDir(flag) {
			return Dir{Path: flag, Kind: KindFlag}
		}
		log.Printf("mimic: -fonts %s is not a folder, ignoring it", flag)
	}
	for _, cand := range CheckoutCandidates {
		if isDir(cand) {
			return Dir{Path: cand, Kind: KindCheckout}
		}
	}
	if managed != "" {
		return Dir{Path: managed, Kind: KindManaged}
	}
	return Dir{Kind: KindNone}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isFontFile reports whether a file name has an extension the engine reads
func isFontFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ttf", ".otf":
		return true
	}
	return false
}

// Add stores a font directly in the folder under its own file name, replacing a
// file of that name. The file must parse as a font before anything on disk
// changes, and it is written beside the old one then renamed, so a failure never
// loses the font it replaces. It returns the stored path
func (d Dir) Add(name string, r io.Reader) (string, error) {
	if !d.Writable() {
		return "", ErrReadOnly
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if !isFontFile(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("%s is not a .ttf or .otf file", name)
	}
	raw, err := io.ReadAll(io.LimitReader(r, MaxFontBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > MaxFontBytes {
		return "", fmt.Errorf("%s is larger than %d MB", name, MaxFontBytes>>20)
	}
	if err := fonts.Validate(raw); err != nil {
		return "", fmt.Errorf("%s is not a font the engine can read: %w", name, err)
	}
	if err := os.MkdirAll(d.Path, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(d.Path, ".upload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	dest := filepath.Join(d.Path, name)
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Remove deletes a font file the folder holds, given the path the pool reports
// for it. A path outside the folder, or one that is not a font file, is refused,
// and a font already gone is not an error
func (d Dir) Remove(path string) error {
	if !d.Writable() {
		return ErrReadOnly
	}
	rel, err := filepath.Rel(d.Path, filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !isFontFile(rel) {
		return fmt.Errorf("%s is not a font in the fonts folder", filepath.Base(path))
	}
	if err := os.Remove(filepath.Join(d.Path, rel)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// RoleFolders are the subfolders an earlier version of the app kept one font per
// role in, by the role each held. Fonts are now kept directly in the folder and
// assigned by name, so FlattenRoleFolders moves them out
var RoleFolders = map[string]string{
	"title": "title", "body": "body", "body-italic": "body-italic",
	"mana": "mana", "type": "small-caps", "info": "info",
}

// Moved is a font taken out of a role folder and where it went
type Moved struct {
	// Role is the role the folder was for, and Path where the font is now. Used
	// is true for the font the role drew with, the first in its folder
	Role string
	Path string
	Used bool
}

// FlattenRoleFolders moves every font in a role folder of the app's own folder up
// into the folder itself, so a font added before roles were assigned by name is
// kept. A font whose name is taken goes up under the role's name in front, and one
// that still cannot be moved stays where it is. A folder left empty is removed,
// so a second call finds nothing to do
func (d Dir) FlattenRoleFolders() []Moved {
	if !d.Writable() {
		return nil
	}
	var moved []Moved
	for _, folder := range []string{"title", "body", "body-italic", "mana", "type", "info"} {
		role := RoleFolders[folder]
		dir := filepath.Join(d.Path, folder)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		used := true
		for _, e := range entries {
			if e.IsDir() || !isFontFile(e.Name()) || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dest := filepath.Join(d.Path, e.Name())
			if _, err := os.Lstat(dest); err == nil {
				dest = filepath.Join(d.Path, folder+"-"+e.Name())
				if _, err := os.Lstat(dest); err == nil {
					continue
				}
			}
			if err := os.Rename(filepath.Join(dir, e.Name()), dest); err != nil {
				log.Printf("mimic: could not move %s out of %s: %v", e.Name(), folder, err)
				continue
			}
			moved = append(moved, Moved{Role: role, Path: dest, Used: used})
			used = false
		}
		// Only an empty folder goes, so anything else in it stays
		_ = os.Remove(dir)
	}
	return moved
}

// Ensure creates the directory when the app owns it, so it can be shown in the
// file browser before any font was added
func (d Dir) Ensure() error {
	if d.Path == "" {
		return errors.New("there is no fonts folder")
	}
	if !d.Writable() {
		return nil
	}
	return os.MkdirAll(d.Path, 0o755)
}

// Pool keeps the engine's index of a font folder. The index is read once and
// kept until Refresh, so a render does not walk the folder, and whatever changes
// the folder asks for a new one
type Pool struct {
	dir  Dir
	mu   sync.Mutex
	pool *fonts.Pool
}

// NewPool returns the index of dir, which is read on first use
func NewPool(dir Dir) *Pool { return &Pool{dir: dir} }

// Get returns the current index
func (p *Pool) Get() *fonts.Pool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pool == nil {
		p.pool = fonts.NewPool(p.dir.Path)
	}
	return p.pool
}

// Refresh reads the folder again and returns the new index
func (p *Pool) Refresh() *fonts.Pool {
	next := fonts.NewPool(p.dir.Path)
	p.mu.Lock()
	p.pool = next
	p.mu.Unlock()
	return next
}
