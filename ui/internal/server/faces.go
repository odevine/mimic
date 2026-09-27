package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
)

func (s *Server) handleFaceTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.pipe.FaceRows())
}

// faceChoiceBody is the PUT /api/template/faces payload. An empty Name clears
// the shape's preference
type faceChoiceBody struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// handleSetFaceTemplate saves the preferred template for one face shape, or
// for standard single cards switches the active template, the same choice the
// top bar makes. It waits for a run to finish, so every face of a run renders
// through the templates it started with
func (s *Server) handleSetFaceTemplate(w http.ResponseWriter, r *http.Request) {
	var body faceChoiceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad face template request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if s.runActive() {
		http.Error(w, "face templates cannot change while a run is in progress", http.StatusConflict)
		return
	}
	var row *pipeline.FaceRow
	for _, fr := range s.pipe.FaceRows() {
		if fr.Key == body.Key {
			row = &fr
			break
		}
	}
	if row == nil {
		http.Error(w, "no installed template renders "+body.Key, http.StatusBadRequest)
		return
	}
	if body.Name != "" && !slices.ContainsFunc(row.Options, func(o pipeline.FaceOption) bool {
		return o.Name == body.Name && (body.Version == "" || slices.Contains(o.Versions, body.Version))
	}) {
		http.Error(w, fmt.Sprintf("%s %s is not an installed template for %s", body.Name, body.Version, body.Key), http.StatusBadRequest)
		return
	}
	if row.Primary {
		if err := s.switchActive(body.Name, body.Version); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, s.pipe.FaceRows())
		return
	}
	s.prefs.SetFaceTemplate(body.Key, prefs.TemplateChoice{Name: body.Name, Version: body.Version})
	writeJSON(w, s.pipe.FaceRows())
}

// switchActive makes an installed template the active one, as choosing it in
// the top bar would. An empty version is the newest installed. It never
// downloads, since the settings list offers only installed versions
func (s *Server) switchActive(name, version string) error {
	if name == "" {
		return fmt.Errorf("standard cards need a template")
	}
	if version == "" {
		v := pipeline.InstalledVersions(name)
		if len(v) == 0 {
			return fmt.Errorf("%s is not installed", name)
		}
		version = v[0]
	}
	at, err := pipeline.FromVersion(context.Background(), name, version, nil)
	if err != nil {
		return err
	}
	s.setActiveTemplate(at)
	return nil
}
