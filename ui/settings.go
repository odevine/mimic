package main

import (
	"encoding/json"
	"net/http"
)

// The themes a settings write may carry. Anything else is stored as the default
var themes = map[string]bool{"dark": true, "light": true, "system": true}

// handleCapabilities returns the gate map the frontend applies to every
// feature-keyed control
func (s *server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, capabilities())
}

// handleSettings returns the interface settings
func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.prefs.settings())
}

// handlePutSettings replaces the interface settings and returns what was kept
func (s *server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body uiSettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad settings request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !themes[body.Theme] {
		body.Theme = ""
	}
	s.prefs.setSettings(body)
	writeJSON(w, s.prefs.settings())
}
