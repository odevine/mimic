package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// prefsData is the on-disk shape: the recent-search list, the last chosen
// template, the two render resolutions, and the interface settings, so the next
// launch restores them all. The resolutions are stored as dpi rather than
// pixels, since dpi means the same thing across templates authored at different
// canvas sizes
type prefsData struct {
	RecentSearches  []string `json:"recentSearches"`
	TemplateName    string   `json:"templateName"`
	TemplateVersion string   `json:"templateVersion"`
	PreviewDPI      int      `json:"previewDpi,omitempty"`
	OutputDPI       int      `json:"outputDpi,omitempty"`
	uiSettings
}

// uiSettings is the part of prefs the browser reads and writes whole through
// /api/settings. Splits holds each flow's region widths in pixels, keyed by
// flow name
type uiSettings struct {
	// Theme is "dark", "light", or "system". Empty reads as dark, the default
	Theme string `json:"theme,omitempty"`
	// ExpandPrintings lists every printing as its own search result rather than
	// collapsing them to one row per card name
	ExpandPrintings bool                 `json:"expandPrintings,omitempty"`
	Splits          map[string][]float64 `json:"splits,omitempty"`
}

// prefs persists a little user state to a JSON file, guarded by a mutex. It
// replaces fyne.Preferences. Reads and writes are best-effort and never fatal,
// matching the desktop app's behavior, so a missing or unwritable config
// directory just means nothing is remembered between launches
type prefs struct {
	mu   sync.Mutex
	path string
	data prefsData
}

// prefsPath is where prefs.json lives, beside the bundle cache under the per-OS
// user config directory so config and cache sit together. It reuses the
// userConfigDir var, so a test can redirect it the same way the cache does
func prefsPath() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "mimic", "prefs.json"), nil
}

// loadPrefs reads prefs.json, returning an empty set when it is missing or
// unreadable. A load failure is never fatal
func loadPrefs() *prefs {
	p := &prefs{}
	path, err := prefsPath()
	if err != nil {
		return p
	}
	p.path = path
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &p.data)
	}
	return p
}

// save writes the current state, best-effort. The caller holds the mutex
func (p *prefs) save() {
	if p.path == "" {
		return
	}
	raw, err := json.MarshalIndent(p.data, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(p.path, raw, 0o644)
}

// recentSearches returns the stored recent-search list
func (p *prefs) recentSearches() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.data.RecentSearches...)
}

// setRecentSearches stores the recent-search list and persists it
func (p *prefs) setRecentSearches(list []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data.RecentSearches = append([]string(nil), list...)
	p.save()
}

// template returns the last chosen template name and version
func (p *prefs) template() (name, version string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.data.TemplateName, p.data.TemplateVersion
}

// setTemplate stores the chosen template name and version and persists them
func (p *prefs) setTemplate(name, version string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data.TemplateName = name
	p.data.TemplateVersion = version
	p.save()
}

// resolution returns the stored preview and output dpi. A zero means nothing
// was chosen yet, which the caller resolves against the active template
func (p *prefs) resolution() (preview, output int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.data.PreviewDPI, p.data.OutputDPI
}

// setResolution stores the two render resolutions and persists them
func (p *prefs) setResolution(preview, output int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data.PreviewDPI = preview
	p.data.OutputDPI = output
	p.save()
}

// settings returns the interface settings
func (p *prefs) settings() uiSettings {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.data.uiSettings
}

// setSettings stores the interface settings and persists them
func (p *prefs) setSettings(u uiSettings) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data.uiSettings = u
	p.save()
}
