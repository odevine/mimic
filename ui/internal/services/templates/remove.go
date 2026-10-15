package templates

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
)

// RemoveRequest names a version of an installed template to take out of the library
type RemoveRequest struct {
	Template string `json:"template"`
	Version  string `json:"version"`
}

// RemoveResult says what was removed. Active says it was the template the app was drawing
// with, which the app has switched away from, and Now is the template it draws with after
type RemoveResult struct {
	Template string `json:"template"`
	Version  string `json:"version"`
	Active   bool   `json:"active"`
	Now      string `json:"now,omitempty"`
}

// Remove deletes a version of an installed template from the cache it is read from, whether it
// was downloaded, imported or made available from the editor. A version that is in use is
// switched away from first, to the newest version of the same template that is left or else the
// template the app starts with. A loose developer folder is not the app's to delete, and a
// choice of template for a face that named the version simply stops applying
func (s *Service) Remove(req RemoveRequest) (RemoveResult, error) {
	if s.ws.Run.Active() {
		return RemoveResult{}, apierr.New(apierr.Conflict, "Templates cannot change while a run is in progress")
	}
	id, ver := req.Template, req.Version
	switch {
	case !pipeline.ValidID(id):
		return RemoveResult{}, apierr.Newf(apierr.BadRequest, "%q is not a template", id)
	case ver == pipeline.LocalVersion:
		return RemoveResult{}, apierr.New(apierr.BadRequest, "A developer folder is not kept by the app, so it is removed by deleting the folder")
	case ver == "" || ver == "." || ver == ".." || strings.ContainsAny(ver, `/\`):
		return RemoveResult{}, apierr.Newf(apierr.BadRequest, "%q is not a version", ver)
	}
	path, err := catalog.BundlePath(id, ver)
	if err != nil {
		return RemoveResult{}, apierr.Wrap(apierr.Internal, "Could not find the folder for templates", err)
	}
	if _, err := os.Stat(path); err != nil {
		return RemoveResult{}, apierr.Newf(apierr.NotFound, "%s %s is not installed", id, ver)
	}
	res := RemoveResult{Template: id, Version: ver}
	if name, current := s.ws.Active(); name == id && current == ver {
		// The bundle is still there while the other is opened, so a failure leaves nothing changed
		at, err := s.fallback(id, ver)
		if err != nil {
			return RemoveResult{}, apierr.Wrap(apierr.Internal, "Could not switch to another template", err)
		}
		s.ws.SetActiveTemplate(at)
		res.Active, res.Now = true, at.Name
	}
	s.ws.Pipe.Forget(id)
	if err := os.Remove(path); err != nil {
		return RemoveResult{}, apierr.Wrap(apierr.Internal, "Could not delete the template", err)
	}
	// The folder goes with its last bundle, and stays when anything else is in it
	_ = os.Remove(filepath.Dir(path))
	return res, nil
}

// fallback opens what the app draws with when a version is removed: the newest other cached
// version of the same template, or else the template the app starts with, or the placeholder
func (s *Service) fallback(id, removed string) (*pipeline.Template, error) {
	versions, _ := catalog.CachedVersions(id)
	for _, v := range versions {
		if v == removed {
			continue
		}
		if at, err := pipeline.FromVersion(context.Background(), id, v, nil); err == nil {
			return at, nil
		}
	}
	// The starting template, unless it is the one being removed and only its bundle is left
	if id == pipeline.DefaultTemplate && pipeline.LooseDir(id) == "" {
		return pipeline.Placeholder()
	}
	at, _, err := pipeline.Resolve(pipeline.DefaultTemplate)
	return at, err
}
