package server

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/ui/internal/fontdir"
)

// fontRole is one role's line in the settings panel
type fontRole struct {
	// Role is the manifest name and Folder the override subfolder, which is
	// also the key the add and remove routes take
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

// fontsStatus is the whole Fonts row: where fonts come from and each role
type fontsStatus struct {
	Kind     string     `json:"kind"`
	Path     string     `json:"path,omitempty"`
	Writable bool       `json:"writable"`
	Roles    []fontRole `json:"roles"`
}

// managedFontsDir is the app's own fonts folder beside the template cache, or
// "" when there is no config directory
func managedFontsDir() string {
	base, err := userConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "mimic", "fonts")
}

// fontsStatus inspects every role against the active directory
func (s *Server) fontsStatus() fontsStatus {
	out := fontsStatus{Kind: string(s.fonts.Kind), Path: s.fonts.Path, Writable: s.fonts.Writable(), Roles: []fontRole{}}
	for _, info := range fonts.Roles() {
		res := fonts.Inspect(info.Role, s.fonts.Path)
		line := fontRole{
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

// handleFonts reports the active fonts folder and what each role draws with
func (s *Server) handleFonts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.fontsStatus())
}

// handlePutFont stores an uploaded font for one role, replacing what it held.
// The upload is a multipart form with the font in its "file" field
func (s *Server) handlePutFont(w http.ResponseWriter, r *http.Request) {
	if !s.fonts.Writable() {
		http.Error(w, fontdir.ErrReadOnly.Error(), http.StatusConflict)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, fontdir.MaxFontBytes+1<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "reading the upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	if _, err := s.fonts.Add(r.PathValue("folder"), header.Filename, file); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, s.fontsStatus())
}

// handleDeleteFont removes one role's font so it returns to its default
func (s *Server) handleDeleteFont(w http.ResponseWriter, r *http.Request) {
	if err := s.fonts.Remove(r.PathValue("folder")); err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, fontdir.ErrReadOnly) {
			code = http.StatusConflict
		}
		http.Error(w, err.Error(), code)
		return
	}
	writeJSON(w, s.fontsStatus())
}

// handleOpenFonts shows the active fonts folder in the system file browser,
// creating the app's own folder first so there is something to show
func (s *Server) handleOpenFonts(w http.ResponseWriter, r *http.Request) {
	if err := s.fonts.Ensure(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := s.open(s.fonts.Path); err != nil {
		http.Error(w, "opening the folder: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
