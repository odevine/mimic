// Package templates is the service behind the template manager and the face
// template settings
package templates

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/progress"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// Service lists, downloads and switches templates
type Service struct{ ws *workspace.Workspace }

// New returns the service over a workspace
func New(ws *workspace.Workspace) *Service { return &Service{ws: ws} }

// Startup keeps the default template up to date in the background so launch
// never blocks on a large download. A loose developer directory and an
// explicitly restored selection are the user's choice, so neither is
// auto-updated
func (s *Service) Startup() {
	if src := s.ws.Startup.Source; src == pipeline.SourceBundle || src == pipeline.SourcePlaceholder {
		go s.updateInBackground(s.ws.Startup.Name)
	}
}

// updateInBackground downloads the latest compatible version of a template and
// swaps it in. It runs at startup, so any failure is logged and the app keeps
// rendering from whatever it resolved to. The page picks up the new active
// template on its next read, and its next render uses it
func (s *Service) updateInBackground(name string) {
	at, err := pipeline.Latest(context.Background(), name)
	if err != nil {
		log.Printf("mimic: template update skipped: %v", err)
		return
	}
	s.ws.SetActiveTemplate(at)
}

// VersionView is one version row as the frontend renders it. Action is the
// trailing control the desktop app chose: empty when the row shows "active" or a
// reason, "select" for a cached version, "download" for one that must be fetched
type VersionView struct {
	Version    string `json:"version"`
	Label      string `json:"label"`
	Cached     bool   `json:"cached"`
	Selectable bool   `json:"selectable"`
	Active     bool   `json:"active"`
	Reason     string `json:"reason,omitempty"`
	Action     string `json:"action"`
}

// TemplateView is one template block: its name, its description, whether this
// build can render it, an optional reason, and its versions
type TemplateView struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Renderable  bool   `json:"renderable"`
	// Standard reports whether the template renders standard single cards,
	// which is what selecting it as the active template is for. A template
	// that does not, such as transform, is only installed
	Standard bool          `json:"standard"`
	Reason   string        `json:"reason,omitempty"`
	Versions []VersionView `json:"versions"`
}

// List fetches the catalog (falling back to the cached copy offline) and returns
// the manager rows, with each version's display label, active flag, and trailing
// action resolved here so the frontend just renders them
func (s *Service) List(ctx context.Context) []TemplateView {
	idx, err := catalog.FetchIndex(ctx)
	if err != nil {
		// No live catalog and no cache: still show local/registered rows
		idx = nil
	}
	activeName, activeVersion := s.ws.Active()
	rows := buildTemplateRows(idx, template.List(), func(n string) bool { return pipeline.LooseDir(n) != "" }, catalog.IsCached)

	views := make([]TemplateView, 0, len(rows))
	for _, row := range rows {
		tv := TemplateView{Name: row.name, Description: row.description, Renderable: row.renderable, Reason: row.reason, Standard: pipeline.SupportsOf(row.name).Allows(pipeline.PrimaryShape)}
		for _, v := range row.versions {
			vv := VersionView{
				Version:    v.version,
				Label:      versionLabel(v.version),
				Cached:     v.cached,
				Selectable: v.selectable,
				Active:     row.name == activeName && v.version == activeVersion,
				Reason:     v.reason,
			}
			switch {
			case vv.Active, !v.selectable:
				vv.Action = ""
			case v.cached:
				vv.Action = "select"
			default:
				vv.Action = "download"
			}
			tv.Versions = append(tv.Versions, vv)
		}
		views = append(views, tv)
	}
	return views
}

// ActiveView is the active template: its name, version, the display label for
// the top-bar indicator, where its assets come from for the status bar, and the
// faces it supports. Faces maps every face shape some installed template renders
// to the template chosen for it, which the list review and the editor check
// cards against
type ActiveView struct {
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	Label    string            `json:"label"`
	Source   string            `json:"source"`
	Supports template.Supports `json:"supports"`
	Faces    map[string]string `json:"faces"`
}

// Active returns the active template
func (s *Service) Active() ActiveView {
	name, version := s.ws.Active()
	return ActiveView{
		Name:     name,
		Version:  version,
		Label:    pipeline.Display(name, version),
		Source:   templateSource(name, version),
		Supports: s.ws.Pipe.Supports(),
		Faces:    s.ws.Pipe.FaceTemplates(),
	}
}

// templateSource names where the active template's assets live: a cached
// bundle, a loose developer directory, or the placeholder layers
func templateSource(name, version string) string {
	switch {
	case version == "":
		return "placeholder"
	case version == pipeline.LocalVersion:
		return "local"
	case catalog.IsCached(name, version):
		return "cached"
	default:
		return "bundle"
	}
}

// SelectRequest names the template version to switch to. Install downloads the
// version without making it the active template, for a template that renders
// only other faces
type SelectRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Install bool   `json:"install,omitempty"`
}

// Select starts a template switch and returns its job. The switch runs in a
// goroutine that emits download progress when a fetch is needed and ends with a
// done event
func (s *Service) Select(req SelectRequest) (string, error) {
	if req.Name == "" || req.Version == "" {
		return "", apierr.New(apierr.BadRequest, "name and version are required")
	}
	if s.ws.Run.Active() {
		return "", apierr.New(apierr.Conflict, "the template cannot change while a run is in progress")
	}
	j := s.ws.Jobs.New()
	go s.doSelect(j, req.Name, req.Version, req.Install)
	return j.ID(), nil
}

// doSelect switches the active template to (name, version), downloading first
// when the version is not cached and emitting download progress. With install set
// it only downloads, and the active template stays
func (s *Service) doSelect(j *jobs.Job, name, version string, install bool) {
	var report func(done, total int64)
	if !catalog.IsCached(name, version) && version != pipeline.LocalVersion {
		report = progress.Bytes(func(step string, frac float64) {
			j.Emit(jobs.Event{Step: step, Frac: frac})
		})
	}
	if install {
		if version == pipeline.LocalVersion {
			j.Emit(jobs.Event{Done: true})
			return
		}
		if _, err := catalog.EnsureVersion(context.Background(), name, version, report); err != nil {
			j.Emit(jobs.Event{Done: true, Err: err.Error()})
			return
		}
		j.Emit(jobs.Event{Done: true})
		return
	}
	at, err := pipeline.FromVersion(context.Background(), name, version, report)
	if err != nil {
		j.Emit(jobs.Event{Done: true, Err: err.Error()})
		return
	}
	s.ws.SetActiveTemplate(at)
	j.Emit(jobs.Event{Done: true})
}

// Faces lists, for each face shape, the installed templates that can render it
// and the one chosen
func (s *Service) Faces() []pipeline.FaceRow { return s.ws.Pipe.FaceRows() }

// FaceChoice is the preferred template for one face shape. An empty Name clears
// the shape's preference
type FaceChoice struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// SetFace saves the preferred template for one face shape, or for standard
// single cards switches the active template, the same choice the top bar makes.
// It waits for a run to finish, so every face of a run renders through the
// templates it started with
func (s *Service) SetFace(c FaceChoice) ([]pipeline.FaceRow, error) {
	if s.ws.Run.Active() {
		return nil, apierr.New(apierr.Conflict, "face templates cannot change while a run is in progress")
	}
	var row *pipeline.FaceRow
	for _, fr := range s.ws.Pipe.FaceRows() {
		if fr.Key == c.Key {
			row = &fr
			break
		}
	}
	if row == nil {
		return nil, apierr.New(apierr.BadRequest, "no installed template renders "+c.Key)
	}
	if c.Name != "" && !slices.ContainsFunc(row.Options, func(o pipeline.FaceOption) bool {
		return o.Name == c.Name && (c.Version == "" || slices.Contains(o.Versions, c.Version))
	}) {
		return nil, apierr.Newf(apierr.BadRequest, "%s %s is not an installed template for %s", c.Name, c.Version, c.Key)
	}
	if row.Primary {
		if err := s.switchActive(c.Name, c.Version); err != nil {
			return nil, apierr.Wrap(apierr.BadRequest, "", err)
		}
		return s.ws.Pipe.FaceRows(), nil
	}
	s.ws.Prefs.SetFaceTemplate(c.Key, prefs.TemplateChoice{Name: c.Name, Version: c.Version})
	return s.ws.Pipe.FaceRows(), nil
}

// switchActive makes an installed template the active one, as choosing it in the
// top bar would. An empty version is the newest installed. It never downloads,
// since the settings list offers only installed versions
func (s *Service) switchActive(name, version string) error {
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
	s.ws.SetActiveTemplate(at)
	return nil
}
