// Package fontdir picks the one font override directory the app renders with
// and manages the app's own fonts folder, the writable one the settings panel
// adds fonts to and removes them from
package fontdir

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

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
	// KindNone means there is no directory at all, so every role uses its default
	KindNone Kind = "none"
)

// CheckoutCandidates are where a checkout's local-fonts folder sits relative
// to a repo-root run and a ui-subdir run
var CheckoutCandidates = []string{"local-fonts", filepath.Join("..", "local-fonts")}

// MaxFontBytes bounds an added font. Real card fonts are well under 1 MB
const MaxFontBytes = 20 << 20

// ErrReadOnly is returned when writing to a directory the app does not own
var ErrReadOnly = errors.New("the active fonts folder is not the app's own, so it is read-only")

// Dir is the active font override directory. Path is "" for KindNone
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

// roleInfo finds the role whose override folder is folder
func roleInfo(folder string) (fonts.RoleInfo, bool) {
	for _, info := range fonts.Roles() {
		if info.Folder == folder {
			return info, true
		}
	}
	return fonts.RoleInfo{}, false
}

// Add stores a font for the role whose folder is folder, replacing whatever the
// folder held. The file must parse as a font before anything on disk changes,
// and it is written beside the old one then renamed, so a failure never leaves
// the role empty. It returns the stored path
func (d Dir) Add(folder, name string, r io.Reader) (string, error) {
	if !d.Writable() {
		return "", ErrReadOnly
	}
	if _, ok := roleInfo(folder); !ok {
		return "", fmt.Errorf("unknown font role %q", folder)
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".ttf" && ext != ".otf" {
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

	dir := filepath.Join(d.Path, folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
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
	dest := filepath.Join(dir, name)
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return "", err
	}
	// The managed folder keeps one font per role, so the new file is the one used
	if err := removeFonts(dir, dest); err != nil {
		return dest, err
	}
	return dest, nil
}

// Remove deletes the role's fonts so it returns to its default. A role with no
// font is not an error
func (d Dir) Remove(folder string) error {
	if !d.Writable() {
		return ErrReadOnly
	}
	if _, ok := roleInfo(folder); !ok {
		return fmt.Errorf("unknown font role %q", folder)
	}
	return removeFonts(filepath.Join(d.Path, folder), "")
}

// removeFonts deletes every .ttf and .otf in dir except keep
func removeFonts(dir, keep string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || path == keep || (ext != ".ttf" && ext != ".otf") {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
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
