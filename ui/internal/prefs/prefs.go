// Package prefs persists the UI's user state to a JSON file: recent searches,
// the chosen template, render resolutions, per-face template preferences, and
// the settings the browser reads and writes whole
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// data is the on-disk shape: the recent-search list, the last chosen
// template, the two render resolutions, and the interface settings, so the next
// launch restores them all. The resolutions are stored as dpi rather than
// pixels, since dpi means the same thing across templates authored at different
// canvas sizes
type data struct {
	RecentSearches  []string `json:"recentSearches"`
	TemplateName    string   `json:"templateName"`
	TemplateVersion string   `json:"templateVersion"`
	PreviewDPI      int      `json:"previewDpi,omitempty"`
	OutputDPI       int      `json:"outputDpi,omitempty"`
	// FaceTemplates is the preferred template for each face shape other than
	// a standard single card, which TemplateName holds, keyed like
	// "transform_front/standard"
	FaceTemplates map[string]TemplateChoice `json:"faceTemplates,omitempty"`
	// CardbackName names the uploaded MPC Autofill cardback, which is kept
	// beside this file
	CardbackName string `json:"cardbackName,omitempty"`
	// Window is where the main window was last, restored on launch
	Window *Window `json:"window,omitempty"`
	// LastUpdateCheck is when the app last asked for a newer release, in Unix
	// seconds, so the check at launch runs at most once a day
	LastUpdateCheck int64 `json:"lastUpdateCheck,omitempty"`
	Settings
}

// Window is the main window's last size and position in screen coordinates, and
// whether it was maximised. A maximised window keeps the size it had before, so
// restoring it later has somewhere to go
type Window struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised,omitempty"`
}

// TemplateChoice names one template and version, the value a face preference
// holds. An empty Version means the newest installed one
type TemplateChoice struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Settings is the part of prefs the browser reads and writes whole through
// /api/settings. Splits holds each flow's region widths in pixels, keyed by
// flow name
type Settings struct {
	// Theme is "dark", "light", or "system". Empty reads as dark, the default
	Theme string `json:"theme,omitempty"`
	// ExpandPrintings lists every printing as its own search result rather than
	// collapsing them to one row per card name
	ExpandPrintings bool `json:"expandPrintings,omitempty"`
	// LivePreview redraws the single card preview as its fields are edited.
	// Nil reads as on, so settings saved before this existed keep it
	LivePreview *bool                `json:"livePreview,omitempty"`
	Splits      map[string][]float64 `json:"splits,omitempty"`
	// Concurrency is how many cards a batch renders at once, zero to size it to
	// the computer
	Concurrency int `json:"concurrency,omitempty"`
	// ImageFormat is the format a batch writes its cards in, "jpeg" or "png".
	// Empty reads as jpeg
	ImageFormat string `json:"imageFormat,omitempty"`
	// PNGCompression is "fast" or "balanced" and applies to PNG output. Empty
	// reads as balanced
	PNGCompression string `json:"pngCompression,omitempty"`
	// LayerCache is how much decoded frame layer data a run keeps in memory to
	// share between cards: "small", "medium", "large" or "xlarge". Empty reads as
	// medium
	LayerCache string `json:"layerCache,omitempty"`
	// OutputDir is the folder a batch writes to, and RecentOutputDirs the
	// folders offered beside it, newest first
	OutputDir        string   `json:"outputDir,omitempty"`
	RecentOutputDirs []string `json:"recentOutputDirs,omitempty"`
	// CardData is where lookups read cards from, "api" or "local". Empty reads
	// as the API
	CardData string `json:"cardData,omitempty"`
	// OutputFormat is "mpc" for an MPC Autofill project, anything else for
	// loose PNGs. MPCStock and MPCFoil are the project's last chosen options
	OutputFormat string `json:"outputFormat,omitempty"`
	MPCStock     string `json:"mpcStock,omitempty"`
	MPCFoil      bool   `json:"mpcFoil,omitempty"`
	// CheckUpdates asks GitHub for a newer release when the app starts. Nil
	// reads as on, so settings saved before this existed keep it
	CheckUpdates *bool `json:"checkUpdates,omitempty"`
	// NotifyRunDone posts a system notification when a run finishes while the
	// window is not in front. Nil reads as on
	NotifyRunDone *bool `json:"notifyRunDone,omitempty"`
	// DismissedUpdate is the release whose banner was closed, so the banner
	// stays away until a newer one comes out
	DismissedUpdate string `json:"dismissedUpdate,omitempty"`
}

// On reports whether a setting that reads as on when unset is on
func On(b *bool) bool { return b == nil || *b }

// Store persists a little user state to a JSON file, guarded by a mutex. Reads
// and writes are best-effort and never fatal, so a missing or unwritable config
// directory just means nothing is remembered between launches
type Store struct {
	mu   sync.Mutex
	path string
	data data
}

// Load reads the prefs file at path, returning an empty set when it is missing
// or unreadable. An empty path gives a store that never persists
func Load(path string) *Store {
	s := &Store{path: path}
	if path == "" {
		return s
	}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &s.data)
	}
	return s
}

// save writes the current state, best-effort. The caller holds the mutex
func (s *Store) save() {
	if s.path == "" {
		return
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(s.path, raw, 0o644)
}

// RecentSearches returns the stored recent-search list
func (s *Store) RecentSearches() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.data.RecentSearches...)
}

// SetRecentSearches stores the recent-search list and persists it
func (s *Store) SetRecentSearches(list []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.RecentSearches = append([]string(nil), list...)
	s.save()
}

// Template returns the last chosen template name and version
func (s *Store) Template() (name, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.TemplateName, s.data.TemplateVersion
}

// SetTemplate stores the chosen template name and version and persists them
func (s *Store) SetTemplate(name, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.TemplateName = name
	s.data.TemplateVersion = version
	s.save()
}

// Resolution returns the stored preview and output dpi. A zero means nothing
// was chosen yet, which the caller resolves against the active template
func (s *Store) Resolution() (preview, output int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.PreviewDPI, s.data.OutputDPI
}

// SetResolution stores the two render resolutions and persists them
func (s *Store) SetResolution(preview, output int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.PreviewDPI = preview
	s.data.OutputDPI = output
	s.save()
}

// Settings returns the interface settings
func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings
}

// SetSettings stores the interface settings and persists them
func (s *Store) SetSettings(u Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Settings = u
	s.save()
}

// WindowState returns the stored window, or nil when none was saved
func (s *Store) WindowState() *Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Window == nil {
		return nil
	}
	w := *s.data.Window
	return &w
}

// SetWindowState stores the window and persists it
func (s *Store) SetWindowState(w Window) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Window = &w
	s.save()
}

// LastUpdateCheck returns when the app last asked for a newer release, or the
// zero time if it never has
func (s *Store) LastUpdateCheck() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.LastUpdateCheck == 0 {
		return time.Time{}
	}
	return time.Unix(s.data.LastUpdateCheck, 0)
}

// SetLastUpdateCheck records when the app asked for a newer release
func (s *Store) SetLastUpdateCheck(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.LastUpdateCheck = t.Unix()
	s.save()
}

// FaceTemplates returns a copy of the per-shape template preferences
func (s *Store) FaceTemplates() map[string]TemplateChoice {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]TemplateChoice, len(s.data.FaceTemplates))
	for k, v := range s.data.FaceTemplates {
		out[k] = v
	}
	return out
}

// SetFaceTemplate stores the preferred template for one shape and persists
// it. An empty choice clears the preference, so the first installed template
// that supports the shape is used again
func (s *Store) SetFaceTemplate(key string, c TemplateChoice) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Name == "" {
		delete(s.data.FaceTemplates, key)
	} else {
		if s.data.FaceTemplates == nil {
			s.data.FaceTemplates = map[string]TemplateChoice{}
		}
		s.data.FaceTemplates[key] = c
	}
	s.save()
}

// CardbackName returns the uploaded cardback's name
func (s *Store) CardbackName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.CardbackName
}

// SetCardbackName stores the uploaded cardback's name and persists it
func (s *Store) SetCardbackName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.CardbackName = name
	s.save()
}
