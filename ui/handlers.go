package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// routes registers every handler. API routes live under /api; everything else
// serves the embedded static frontend
func (s *server) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/recents", s.handleRecents)
	mux.HandleFunc("POST /api/render", s.handleRender)
	mux.HandleFunc("GET /api/render/{id}/events", s.handleJobEvents)
	mux.HandleFunc("GET /api/render/{id}/image", s.handleRenderImage)
	mux.HandleFunc("GET /api/templates", s.handleTemplates)
	mux.HandleFunc("GET /api/template/active", s.handleActiveTemplate)
	mux.HandleFunc("POST /api/template/select", s.handleSelectTemplate)
	mux.HandleFunc("GET /api/template/select/{id}/events", s.handleJobEvents)
	mux.Handle("/", http.FileServerFS(staticFS()))
	s.mux = mux
}

// searchResult is one row the search endpoint returns: the display line the list
// shows and the full card the client fills its form from and posts back
type searchResult struct {
	Text string     `json:"text"`
	Card *card.Data `json:"card"`
}

// handleSearch runs a Scryfall search and returns the matches. A successful
// search is remembered for the suggestion list
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, []searchResult{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), netTimeout)
	defer cancel()

	cards, err := s.pipe.client.Search(ctx, query)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		http.Error(w, "search failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.rememberQuery(query)

	out := make([]searchResult, len(cards))
	for i, c := range cards {
		out[i] = searchResult{Text: rowText(c), Card: c}
	}
	writeJSON(w, out)
}

// handleRecents returns the recent-search suggestions, newest first
func (s *server) handleRecents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.recentQueries())
}

// renderBody is the /api/render payload: the selected card as fetched, plus the
// editor's current field values overlaid onto it. Sending the base card keeps
// fields the form does not expose (color identity, produced mana, artwork URL)
// intact and lets the art cache key on the unchanged URL
type renderBody struct {
	Base  card.Data  `json:"base"`
	Edits editFields `json:"edits"`
}

// handleRender starts a render job and returns its id. The render runs in a
// goroutine that streams progress; the client watches the events stream and then
// fetches the image
func (s *server) handleRender(w http.ResponseWriter, r *http.Request) {
	var body renderBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad render request: "+err.Error(), http.StatusBadRequest)
		return
	}
	d := applyEdits(&body.Base, body.Edits)

	id, j := s.newJob()
	go s.doRender(j, d)
	writeJSON(w, map[string]string{"jobId": id})
}

// handleRenderImage writes a finished render's PNG. With a download query it
// sets an attachment disposition and the card-name filename, so the Save link
// downloads; otherwise it serves inline for the preview image
func (s *server) handleRenderImage(w http.ResponseWriter, r *http.Request) {
	j, ok := s.lookupJob(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	img, name := j.result()
	if img == nil {
		http.Error(w, "render not finished", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	if r.URL.Query().Has("download") {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", pngFilename(name)))
	}
	if err := png.Encode(w, img); err != nil {
		http.Error(w, "encoding image: "+err.Error(), http.StatusInternalServerError)
	}
}

// handleJobEvents streams a job's events as server-sent events, replaying the
// backlog to a late subscriber and ending after the terminal done event. It
// serves both render and template-download jobs, which share the job map
func (s *server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	j, ok := s.lookupJob(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	from := 0
	for {
		events, finished, wait := j.stream(from)
		from += len(events)
		for _, e := range events {
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		flusher.Flush()
		if finished {
			return
		}
		select {
		case <-wait:
		case <-r.Context().Done():
			return
		}
	}
}

// versionView is one version row as the frontend renders it. Action is the
// trailing control the desktop app chose: empty when the row shows "active" or a
// reason, "select" for a cached version, "download" for one that must be fetched
type versionView struct {
	Version    string `json:"version"`
	Label      string `json:"label"`
	Cached     bool   `json:"cached"`
	Selectable bool   `json:"selectable"`
	Active     bool   `json:"active"`
	Reason     string `json:"reason,omitempty"`
	Action     string `json:"action"`
}

// templateView is one template block: its name, whether this build can render
// it, an optional reason, and its versions
type templateView struct {
	Name       string        `json:"name"`
	Renderable bool          `json:"renderable"`
	Reason     string        `json:"reason,omitempty"`
	Versions   []versionView `json:"versions"`
}

// handleTemplates fetches the catalog (falling back to the cached copy offline)
// and returns the manager rows, with each version's display label, active flag,
// and trailing action resolved server-side so the frontend just renders them
func (s *server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	idx, err := fetchIndex(r.Context())
	if err != nil {
		// No live catalog and no cache: still show local/registered rows
		idx = nil
	}
	activeName, activeVersion := s.active()
	rows := buildTemplateRows(idx, template.Names(), func(n string) bool { return looseDir(n) != "" }, isVersionCached)

	views := make([]templateView, 0, len(rows))
	for _, row := range rows {
		tv := templateView{Name: row.name, Renderable: row.renderable, Reason: row.reason}
		for _, v := range row.versions {
			vv := versionView{
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
	writeJSON(w, views)
}

// handleActiveTemplate returns the active template's name, version, and the
// display label for the top-bar indicator
func (s *server) handleActiveTemplate(w http.ResponseWriter, r *http.Request) {
	name, version := s.active()
	writeJSON(w, map[string]string{
		"name":    name,
		"version": version,
		"label":   templateDisplay(name, version),
	})
}

// selectBody is the /api/template/select payload
type selectBody struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// handleSelectTemplate starts a template switch job and returns its id. The
// switch runs in a goroutine that streams download progress when a fetch is
// needed and ends with a done event
func (s *server) handleSelectTemplate(w http.ResponseWriter, r *http.Request) {
	var body selectBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad select request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.Version == "" {
		http.Error(w, "name and version are required", http.StatusBadRequest)
		return
	}
	id, j := s.newJob()
	go s.doSelectTemplate(j, body.Name, body.Version)
	writeJSON(w, map[string]string{"jobId": id})
}

// writeJSON encodes v as the JSON response body
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
