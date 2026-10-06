package server

import (
	"encoding/json"
	"net/http"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/resource"
)

// The themes a settings write may carry. Anything else is stored as the default
var themes = map[string]bool{"dark": true, "light": true, "system": true}

// maxRecentOutputDirs bounds the recent output folders a settings write keeps
const maxRecentOutputDirs = 6

// handleCapabilities returns the gate map the frontend applies to every
// feature-keyed control
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, capabilities())
}

// handleSettings returns the interface settings
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.prefs.Settings())
}

// handlePutSettings replaces the interface settings and returns what was kept
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body prefs.Settings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad settings request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !themes[body.Theme] {
		body.Theme = ""
	}
	if body.CardData != cardDataLocal {
		body.CardData = ""
	}
	if body.Concurrency < 0 || body.Concurrency > batch.MaxConcurrency {
		body.Concurrency = 0
	}
	if !resource.ValidCacheSize(body.LayerCache) {
		body.LayerCache = ""
	}
	if body.OutputFormat != "mpc" {
		body.OutputFormat = ""
	}
	if len(body.RecentOutputDirs) > maxRecentOutputDirs {
		body.RecentOutputDirs = body.RecentOutputDirs[:maxRecentOutputDirs]
	}
	s.prefs.SetSettings(body)
	pipeline.SetLayerCacheBytes(int64(resource.CacheBytes(s.layerCacheName())))
	writeJSON(w, s.prefs.Settings())
}
