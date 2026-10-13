package data

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/fontdir"
	"github.com/odevine/mimic/ui/internal/pipeline"
)

// FontRole is one text role's line in the settings panel
type FontRole struct {
	// Role is the name a template's text boxes use, and Label how it is shown
	Role  string `json:"role"`
	Label string `json:"label"`
	// Source is how the role resolved, as the engine reports it: override,
	// template, template-missing-override, or fallback
	Source string `json:"source"`
	// Family and Style name the font the role draws with now
	Family string `json:"family,omitempty"`
	Style  string `json:"style,omitempty"`
	// Own is the name of the role's own font, which the role draws with when no
	// font is assigned, and Bundled is true when the template ships it
	Own     string `json:"own"`
	Bundled bool   `json:"bundled,omitempty"`
	// Override is the font assigned to the role, whether or not the folder has
	// it, and File is the file in use when it does
	Override string `json:"override,omitempty"`
	File     string `json:"file,omitempty"`
	Error    string `json:"error,omitempty"`
}

// FontFile is one font in the folder a role can be assigned
type FontFile struct {
	// Name is the family and style joined by a space, which is how a role is
	// assigned the font and how a font is removed
	Name   string `json:"name"`
	Family string `json:"family"`
	Style  string `json:"style"`
	File   string `json:"file"`
	// Removable is true for a font the app may delete, which is one in its own
	// folder
	Removable bool `json:"removable"`
}

// FontReject is a file in the folder that could not be used and why
type FontReject struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

// FontsStatus is the whole Fonts row: where fonts come from, what a role can be
// assigned, and what each role draws with
type FontsStatus struct {
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Writable bool   `json:"writable"`
	// Roles is every role the installed templates have. It is empty until a
	// template is installed
	Roles []FontRole `json:"roles"`
	// Pool is the fonts in the folder, by path
	Pool []FontFile `json:"pool"`
	// Rejected lists files the folder holds that are not fonts the engine can
	// read, and Duplicates the files left out because another has the same name
	Rejected   []FontReject `json:"rejected"`
	Duplicates []string     `json:"duplicates"`
}

// Fonts reports the active fonts folder, reading it again so a font put there by
// hand shows, and what each role draws with
func (s *Service) Fonts() FontsStatus {
	dir := s.ws.Fonts
	pool := s.ws.FontPool.Refresh()
	out := FontsStatus{
		Kind: string(dir.Kind), Path: dir.Path, Writable: dir.Writable(),
		Roles: []FontRole{}, Pool: []FontFile{}, Rejected: []FontReject{}, Duplicates: []string{},
	}
	for _, f := range pool.Fonts() {
		out.Pool = append(out.Pool, FontFile{Name: f.Name, Family: f.Family, Style: f.Style, File: filepath.Base(f.Path), Removable: dir.Writable()})
	}
	for _, r := range pool.Rejected() {
		out.Rejected = append(out.Rejected, FontReject{File: filepath.Base(r.Path), Error: r.Err.Error()})
	}
	for _, p := range pool.Duplicates() {
		out.Duplicates = append(out.Duplicates, filepath.Base(p))
	}
	out.Roles = s.fontRoles(pool)
	return out
}

// fontRoles lists the roles of every installed template once each, the standard
// ones first in the engine's order and then the ones templates define by name,
// each resolved against the template that declares it
func (s *Service) fontRoles(pool *fonts.Pool) []FontRole {
	overrides := s.ws.Prefs.Settings().FontOverrides
	rows := map[string]FontRole{}
	for _, in := range pipeline.InstalledTemplates() {
		cat := in.Template.Fonts()
		resolver := fonts.NewResolver(cat, pool, overrides)
		for _, info := range cat.Roles() {
			if _, seen := rows[info.Name]; seen {
				continue
			}
			res := resolver.Inspect(info.Role)
			row := FontRole{
				Role: info.Name, Label: info.Label, Source: string(res.Source),
				Family: res.Family, Style: res.Style,
				Own: info.Family + " " + info.Style, Bundled: info.Bundled,
				Override: res.Override,
			}
			if res.Source == fonts.SourceOverride {
				row.File = filepath.Base(res.Path)
			}
			if res.Err != nil {
				row.Error = res.Err.Error()
			}
			rows[info.Name] = row
		}
	}
	var out []FontRole
	for _, info := range fonts.Roles() {
		if row, ok := rows[info.Name]; ok {
			out = append(out, row)
			delete(rows, info.Name)
		}
	}
	custom := make([]string, 0, len(rows))
	for name := range rows {
		custom = append(custom, name)
	}
	sort.Strings(custom)
	for _, name := range custom {
		out = append(out, rows[name])
	}
	if out == nil {
		out = []FontRole{}
	}
	return out
}

// AddFont adds the font file at path to the app's fonts folder, replacing one of
// the same file name
func (s *Service) AddFont(path string) (FontsStatus, error) {
	f, err := os.Open(path)
	if err != nil {
		return FontsStatus{}, apierr.Wrap(apierr.BadRequest, "reading the font", err)
	}
	defer f.Close()
	return s.AddFontFile(filepath.Base(path), f)
}

// AddFontFile adds a font read from r under filename to the app's fonts folder,
// replacing one of the same file name
func (s *Service) AddFontFile(filename string, r io.Reader) (FontsStatus, error) {
	if !s.ws.Fonts.Writable() {
		return FontsStatus{}, apierr.Wrap(apierr.Conflict, "", fontdir.ErrReadOnly)
	}
	if _, err := s.ws.Fonts.Add(filename, r); err != nil {
		return FontsStatus{}, apierr.Wrap(apierr.BadRequest, "", err)
	}
	return s.Fonts(), nil
}

// RemoveFont deletes the font with this name, "Family Style", from the app's
// fonts folder. A role assigned the font goes back to its own font and says the
// assigned one is missing
func (s *Service) RemoveFont(name string) (FontsStatus, error) {
	f, ok := s.ws.FontPool.Refresh().Lookup(name)
	if !ok {
		return FontsStatus{}, apierr.New(apierr.NotFound, "that font is not in the fonts folder")
	}
	if err := s.ws.Fonts.Remove(f.Path); err != nil {
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
