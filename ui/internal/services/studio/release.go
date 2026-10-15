package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
)

// ReleaseView is what making a draft into a bundle would produce and say about itself,
// which is what a person is shown before it goes anywhere. Problems are what stops the
// bundle being made, and warnings are listed but do not
type ReleaseView struct {
	TemplateID string            `json:"templateId"`
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Author     string            `json:"author"`
	License    string            `json:"license"`
	Credits    []manifest.Credit `json:"credits"`
	// Filename is what a bundle of this template is called when it is saved
	Filename string `json:"filename"`
	// Installed is whether the draft has been made available to the other modes, and
	// Active whether it is the template they are drawing with now
	Installed   bool             `json:"installed"`
	Active      bool             `json:"active"`
	Diagnostics []DiagnosticView `json:"diagnostics"`
	// Ready is whether the bundle can be made
	Ready bool `json:"ready"`
}

// header is the part of template.json a person is told about before a release
type header struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Author  struct {
		Name string `json:"name"`
	} `json:"author"`
	License string            `json:"license"`
	Credits []manifest.Credit `json:"credits"`
}

func readHeader(fsys fs.FS) header {
	var h header
	raw, err := fs.ReadFile(fsys, "template.json")
	if err == nil {
		_ = json.Unmarshal(raw, &h)
	}
	if h.Credits == nil {
		h.Credits = []manifest.Credit{}
	}
	return h
}

// Release reads a draft as a bundle would carry it, checking that it builds, without
// making the bundle
func (s *Service) Release(id string) (ReleaseView, error) {
	fsys, err := s.store.FS(id)
	if err != nil {
		return ReleaseView{}, err
	}
	h := readHeader(fsys)
	view := ReleaseView{TemplateID: h.ID, Name: h.Name, Version: h.Version, Author: h.Author.Name, License: h.License, Credits: h.Credits}
	if view.TemplateID == "" {
		sum, err := s.store.Get(id)
		if err != nil {
			return ReleaseView{}, err
		}
		view.TemplateID = sum.TemplateID
	}
	view.Filename = bundleName(view.TemplateID, view.Version)
	view.Installed = catalog.IsCached(view.TemplateID, pipeline.DraftVersion)
	name, ver := s.ws.Active()
	view.Active = view.Installed && name == view.TemplateID && ver == pipeline.DraftVersion
	ds := author.Validate(fsys, author.Options{})
	view.Diagnostics = diagnosticViews(ds)
	view.Ready = !ds.HasErrors()
	return view, nil
}

// bundleName is the filename a bundle is saved under by default
func bundleName(id, version string) string {
	if version == "" {
		return id + ".mimic"
	}
	return id + "-" + version + ".mimic"
}

// build makes the bundle a draft describes, or says why it cannot
func (s *Service) build(id string) (*author.Built, string, error) {
	fsys, err := s.store.FS(id)
	if err != nil {
		return nil, "", err
	}
	built, ds := author.Build(fsys, author.Options{})
	if built == nil {
		return nil, "", apierr.New(apierr.BadRequest, "The template cannot be made into a bundle: "+firstProblem(ds, nil))
	}
	return built, built.Header.ID, nil
}

// writeBundle writes a built template to path, so a bundle that is half written is
// never left where one was
func writeBundle(built *author.Built, path string) (int64, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	f, err := os.CreateTemp(dir, ".bundle-*")
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := built.WriteZip(f); err != nil {
		f.Close()
		return 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// UseResult says what the draft is available as
type UseResult struct {
	Template string `json:"template"`
	Version  string `json:"version"`
	// Replaced is whether the draft was already available and has been brought up to
	// date, and Active whether the other modes are drawing with it now
	Replaced bool `json:"replaced"`
	Active   bool `json:"active"`
}

// Use makes a draft available to Single, List and Run as the template user.<name>
// at the version draft, by building it and putting the bundle in the cache the
// installed templates are read from. Using it again after more edits replaces the
// bundle, and the template the app is drawing with is opened again so the next card
// shows the change
func (s *Service) Use(id string) (UseResult, error) {
	if s.ws.Run.Active() {
		return UseResult{}, apierr.New(apierr.Conflict, "The template cannot change while a run is in progress")
	}
	built, tid, err := s.build(id)
	if err != nil {
		return UseResult{}, err
	}
	path, err := catalog.BundlePath(tid, pipeline.DraftVersion)
	if err != nil {
		return UseResult{}, apierr.Wrap(apierr.Internal, "Could not find the folder for templates", err)
	}
	replaced := catalog.IsCached(tid, pipeline.DraftVersion)
	if _, err := writeBundle(built, path); err != nil {
		return UseResult{}, apierr.Wrap(apierr.Internal, "Could not save the template", err)
	}
	res := UseResult{Template: tid, Version: pipeline.DraftVersion, Replaced: replaced}
	if name, ver := s.ws.Active(); name == tid && ver == pipeline.DraftVersion {
		at, err := pipeline.FromVersion(context.Background(), tid, pipeline.DraftVersion, nil)
		if err != nil {
			return UseResult{}, apierr.Wrap(apierr.Internal, "The template was saved but could not be opened", err)
		}
		s.ws.SetActiveTemplate(at)
		res.Active = true
	}
	s.ws.Pipe.Forget(tid)
	return res, nil
}

// withdraw takes a draft's template out of the ones the other modes can use, and falls
// back to the default when it was the one in use
func (s *Service) withdraw(tid string) {
	if !catalog.IsCached(tid, pipeline.DraftVersion) {
		return
	}
	if name, _ := s.ws.Active(); name == tid {
		if at, _, err := pipeline.Resolve(pipeline.DefaultTemplate); err == nil {
			s.ws.SetActiveTemplate(at)
		}
	}
	s.ws.Pipe.Forget(tid)
	path, err := catalog.BundlePath(tid, pipeline.DraftVersion)
	if err != nil {
		return
	}
	_ = os.Remove(path)
	// The folder goes with its last bundle, and stays when anything else is in it
	_ = os.Remove(filepath.Dir(path))
}

// ExportRequest asks for a draft to be written as a bundle at a path the person chose
type ExportRequest struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// ExportResult is where the bundle went and how large it is
type ExportResult struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// Export writes a draft as a .mimic bundle, which carries its license and credits. The
// path gets the extension when it was typed without one
func (s *Service) Export(req ExportRequest) (ExportResult, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return ExportResult{}, apierr.New(apierr.BadRequest, "Choose where to save the bundle")
	}
	if !strings.EqualFold(filepath.Ext(path), ".mimic") {
		path += ".mimic"
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return ExportResult{}, apierr.New(apierr.BadRequest, "That is a folder, and the bundle needs a file name")
	}
	built, _, err := s.build(req.ID)
	if err != nil {
		return ExportResult{}, err
	}
	n, err := writeBundle(built, path)
	if err != nil {
		return ExportResult{}, apierr.Wrap(apierr.Internal, fmt.Sprintf("Could not write %s", filepath.Base(path)), err)
	}
	return ExportResult{Path: path, Bytes: n}, nil
}

// ShowFolder opens the folder that holds a draft's files in the system file browser
func (s *Service) ShowFolder(id string) error {
	p, err := s.store.Path(id)
	if err != nil {
		return err
	}
	if s.show == nil {
		return apierr.New(apierr.Internal, "Folders cannot be opened here")
	}
	if err := s.show(p); err != nil {
		return apierr.Wrap(apierr.Internal, "Could not open the folder", err)
	}
	return nil
}
