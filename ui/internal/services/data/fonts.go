package data

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/fontdir"
)

// FontRole is one role's line in the settings panel
type FontRole struct {
	// Role is the manifest name and Folder the override subfolder, which is
	// also the key AddFont and RemoveFont take
	Role   string `json:"role"`
	Folder string `json:"folder"`
	// Source is user, default, borrowed, or fallback, as the engine reports it
	Source string `json:"source"`
	File   string `json:"file,omitempty"`
	Family string `json:"family,omitempty"`
	Style  string `json:"style,omitempty"`
	// Borrows names the role whose default a borrowed role draws with
	Borrows string `json:"borrows,omitempty"`
	// Rejected is a file that failed to load and Error why
	Rejected string `json:"rejected,omitempty"`
	Error    string `json:"error,omitempty"`
	// Ignored lists further font files the role never uses
	Ignored []string `json:"ignored,omitempty"`
}

// FontsStatus is the whole Fonts row: where fonts come from and each role
type FontsStatus struct {
	Kind     string     `json:"kind"`
	Path     string     `json:"path,omitempty"`
	Writable bool       `json:"writable"`
	Roles    []FontRole `json:"roles"`
}

// Fonts reports the active fonts folder and what each role draws with
func (s *Service) Fonts() FontsStatus {
	dir := s.ws.Fonts
	out := FontsStatus{Kind: string(dir.Kind), Path: dir.Path, Writable: dir.Writable(), Roles: []FontRole{}}
	for _, info := range fonts.Roles() {
		res := fonts.Inspect(info.Role, dir.Path)
		line := FontRole{
			Role: info.Name, Folder: info.Folder, Source: string(res.Source),
			Family: res.Family, Style: res.Style, Borrows: info.Borrows,
		}
		if res.Source == fonts.SourceUser {
			line.File = filepath.Base(res.Path)
		}
		if res.Rejected != "" {
			line.Rejected = filepath.Base(res.Rejected)
			line.Error = res.Err.Error()
		}
		for _, p := range res.Ignored {
			line.Ignored = append(line.Ignored, filepath.Base(p))
		}
		out.Roles = append(out.Roles, line)
	}
	return out
}

// AddFont stores the font file at path for one role, replacing what it held
func (s *Service) AddFont(folder, path string) (FontsStatus, error) {
	f, err := os.Open(path)
	if err != nil {
		return FontsStatus{}, apierr.Wrap(apierr.BadRequest, "reading the font", err)
	}
	defer f.Close()
	return s.AddFontFile(folder, filepath.Base(path), f)
}

// AddFontFile stores a font read from r under filename for one role, replacing
// what it held
func (s *Service) AddFontFile(folder, filename string, r io.Reader) (FontsStatus, error) {
	if !s.ws.Fonts.Writable() {
		return FontsStatus{}, apierr.Wrap(apierr.Conflict, "", fontdir.ErrReadOnly)
	}
	if _, err := s.ws.Fonts.Add(folder, filename, r); err != nil {
		return FontsStatus{}, apierr.Wrap(apierr.BadRequest, "", err)
	}
	return s.Fonts(), nil
}

// RemoveFont removes one role's font so it returns to its default
func (s *Service) RemoveFont(folder string) (FontsStatus, error) {
	if err := s.ws.Fonts.Remove(folder); err != nil {
		kind := apierr.BadRequest
		if errors.Is(err, fontdir.ErrReadOnly) {
			kind = apierr.Conflict
		}
		return FontsStatus{}, apierr.Wrap(kind, "", err)
	}
	return s.Fonts(), nil
}

// OpenFonts shows the active fonts folder in the system file browser, creating
// the app's own folder first so there is something to show
func (s *Service) OpenFonts() error {
	if err := s.ws.Fonts.Ensure(); err != nil {
		return apierr.Wrap(apierr.Conflict, "", err)
	}
	if err := s.open(s.ws.Fonts.Path); err != nil {
		return apierr.Wrap(apierr.Internal, "opening the folder", err)
	}
	return nil
}
