package fakeupstream

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
	_ "github.com/odevine/mimic/engine/template/all" // registers every template
)

// Release is a ui release the fake offers
type Release struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

// installer is the file attached to a release, named the way the updater picks
// it for the platform the fake runs on, which is the one the app runs on
func installer(version string) string {
	switch runtime.GOOS {
	case "darwin":
		return "Mimic-" + version + "-macos-universal.zip"
	case "windows":
		return "Mimic-" + version + "-windows-" + runtime.GOARCH + ".zip"
	default:
		return "Mimic-" + version + "-linux-" + runtime.GOARCH + ".AppImage"
	}
}

// installerBytes is what the installer holds. The app never runs it
func installerBytes(version string) []byte { return []byte("fake installer for ui " + version) }

func releasePage(version string) string {
	return "https://github.com/odevine/mimic/releases/tag/ui/v" + version
}

func downloadBase(version string) string {
	return "https://github.com/odevine/mimic/releases/download/ui/v" + version + "/"
}

func (s *Server) githubAPI(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/releases") {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	rel := s.release
	s.mu.Unlock()
	if rel == nil {
		// No releases, so the updater reports the app as current
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	name := installer(rel.Version)
	body := installerBytes(rel.Version)
	sum := sha256.Sum256(body)
	sums := hex.EncodeToString(sum[:]) + "  " + name + "\n"
	writeJSON(w, http.StatusOK, []any{map[string]any{
		"tag_name":     "ui/v" + rel.Version,
		"name":         "Mimic " + rel.Version,
		"body":         rel.Notes,
		"html_url":     releasePage(rel.Version),
		"draft":        false,
		"prerelease":   false,
		"published_at": time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"assets": []any{
			map[string]any{"name": name, "size": len(body), "browser_download_url": downloadBase(rel.Version) + name},
			map[string]any{"name": "SHA256SUMS", "size": len(sums), "browser_download_url": downloadBase(rel.Version) + "SHA256SUMS"},
		},
	}})
}

// githubFiles serves the files attached to releases: a ui installer and its
// checksums, and the template bundles
func (s *Server) githubFiles(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	rel, catalog := s.release, s.catalog
	s.mu.Unlock()
	file := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if rel != nil && strings.Contains(r.URL.Path, "/odevine/mimic/releases/download/ui/v"+rel.Version+"/") {
		switch file {
		case installer(rel.Version):
			w.Write(installerBytes(rel.Version))
			return
		case "SHA256SUMS":
			sum := sha256.Sum256(installerBytes(rel.Version))
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), installer(rel.Version))
			return
		}
	}
	for _, b := range catalog {
		if file == b.file() {
			w.Header().Set("Content-Type", "application/zip")
			w.Write(b.data)
			return
		}
	}
	http.NotFound(w, r)
}

// bundle is a template bundle the fake offers: the engine's placeholder assets
// with a header, so the app installs it and renders with it
type bundle struct {
	name, version string
	data          []byte
	sha           string
}

func (b bundle) file() string { return b.name + "-" + b.version + ".mimic" }

func (b bundle) url() string {
	return "https://github.com/odevine/mimic-templates/releases/download/" + b.name + "-v" + b.version + "/" + b.file()
}

// placeholders holds each template's generated assets, as path and bytes, since
// generating them is the slow part of building a bundle
var placeholders = struct {
	sync.Mutex
	byName map[string][]placeholder
}{byName: map[string][]placeholder{}}

type placeholder struct {
	path string
	data []byte
}

func placeholderFiles(name string) ([]placeholder, error) {
	placeholders.Lock()
	defer placeholders.Unlock()
	if files, ok := placeholders.byName[name]; ok {
		return files, nil
	}
	if _, err := template.Get(name); err != nil {
		return nil, fmt.Errorf("fakeupstream: no template named %q: %w", name, err)
	}
	dir, err := os.MkdirTemp("", "fakeupstream-bundle-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := render.WritePlaceholderAssets(dir, name); err != nil {
		return nil, err
	}
	var files []placeholder
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		raw, err := os.ReadFile(path)
		files = append(files, placeholder{filepath.ToSlash(rel), raw})
		return err
	})
	if err != nil {
		return nil, err
	}
	placeholders.byName[name] = files
	return files, nil
}

func newBundle(name, version string) (bundle, error) {
	if name == "" || version == "" {
		return bundle{}, fmt.Errorf("fakeupstream: a template needs a name and a version")
	}
	files, err := placeholderFiles(name)
	if err != nil {
		return bundle{}, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	header, _ := json.Marshal(map[string]any{"format": 1, "template": name, "version": version, "minEngine": "0.3.0"})
	w, err := zw.Create("bundle.json")
	if err != nil {
		return bundle{}, err
	}
	w.Write(header)
	for _, f := range files {
		fw, err := zw.Create(f.path)
		if err != nil {
			return bundle{}, err
		}
		if _, err := fw.Write(f.data); err != nil {
			return bundle{}, err
		}
	}
	if err := zw.Close(); err != nil {
		return bundle{}, err
	}
	sum := sha256.Sum256(buf.Bytes())
	return bundle{name: name, version: version, data: buf.Bytes(), sha: hex.EncodeToString(sum[:])}, nil
}

func (s *Server) githubRaw(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/index.json") {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	catalog := s.catalog
	s.mu.Unlock()
	type version struct {
		Version   string `json:"version"`
		MinEngine string `json:"minEngine"`
		URL       string `json:"url"`
		Size      int    `json:"size"`
		SHA256    string `json:"sha256"`
	}
	type entry struct {
		Name     string    `json:"name"`
		Latest   string    `json:"latest"`
		Versions []version `json:"versions"`
	}
	byName := map[string]*entry{}
	var order []string
	for _, b := range catalog {
		e := byName[b.name]
		if e == nil {
			e = &entry{Name: b.name}
			byName[b.name] = e
			order = append(order, b.name)
		}
		// Newest first, so Versions[0] is Latest
		e.Versions = append([]version{{Version: b.version, MinEngine: "0.3.0", URL: b.url(), Size: len(b.data), SHA256: b.sha}}, e.Versions...)
		e.Latest = e.Versions[0].Version
	}
	templates := make([]*entry, 0, len(order))
	for _, n := range order {
		templates = append(templates, byName[n])
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema": 1, "templates": templates})
}
