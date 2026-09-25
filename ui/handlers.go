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
	mux.HandleFunc("GET /api/resolution", s.handleResolution)
	mux.HandleFunc("POST /api/resolution", s.handleSetResolution)
	mux.HandleFunc("GET /api/templates", s.handleTemplates)
	mux.HandleFunc("GET /api/template/active", s.handleActiveTemplate)
	mux.HandleFunc("POST /api/template/select", s.handleSelectTemplate)
	mux.HandleFunc("GET /api/template/select/{id}/events", s.handleJobEvents)
	mux.HandleFunc("GET /api/capabilities", s.handleCapabilities)
	mux.HandleFunc("GET /api/settings", s.handleSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("GET /api/printings", s.handlePrintings)
	mux.HandleFunc("GET /api/symbol", s.handleSymbol)
	mux.HandleFunc("POST /api/resolve", s.handleResolve)
	mux.HandleFunc("GET /api/resolve/{id}/events", s.handleJobEvents)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("GET /api/run", s.handleLatestRun)
	mux.HandleFunc("GET /api/run/{id}/events", s.handleRunEvents)
	mux.HandleFunc("GET /api/run/{id}/image/{n}", s.handleRunImage)
	mux.HandleFunc("POST /api/run/{id}/stop", s.handleStopRun)
	mux.HandleFunc("POST /api/run/{id}/retry", s.handleRetryRun)
	mux.HandleFunc("POST /api/run/{id}/open", s.handleOpenRunFolder)
	mux.HandleFunc("GET /api/fs/list", s.handleFSList)
	mux.HandleFunc("GET /api/carddata", s.handleCardData)
	mux.HandleFunc("POST /api/carddata/download", s.handleCardDataDownload)
	mux.HandleFunc("GET /api/carddata/{id}/events", s.handleJobEvents)
	mux.HandleFunc("DELETE /api/carddata", s.handleCardDataDelete)
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

// printingsQuery is the Scryfall search listing every printing of one card by
// exact name, newest first
func printingsQuery(name string) string {
	return fmt.Sprintf("!%q unique:prints order:released dir:desc", name)
}

// handlePrintings lists every printing of a card, for the editor's printing
// picker. It reads the local copy when that is in use, and otherwise goes
// through Search rather than a set-and-number fetch, so it needs nothing the
// card client does not already do
func (s *server) handlePrintings(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeJSON(w, []*card.Data{})
		return
	}
	// A card newer than the local copy is not in it, so an empty answer still
	// asks Scryfall
	if s.useLocalCards() {
		if cards, err := s.cards.printings(name); err == nil && len(cards) > 0 {
			writeJSON(w, cards)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), netTimeout)
	defer cancel()

	cards, err := s.pipe.client.Search(ctx, printingsQuery(name))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		http.Error(w, "listing printings failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if cards == nil {
		cards = []*card.Data{}
	}
	writeJSON(w, cards)
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
	// Target picks which resolution to render at: "output" for the full-size
	// render behind Save, anything else for the preview
	Target string `json:"target,omitempty"`
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

	// Resolving the dpi once here rather than once in the handler and again in
	// the job keeps the number reported back the one actually rendered, even if
	// the setting changes while this render is in flight
	dpi := s.renderDPI(body.Target)
	if m, err := s.pipe.manifest(); err == nil {
		dpi = m.ClampDPI(dpi)
	}

	id, j := s.newJob()
	go s.doRender(j, d, dpi)
	writeJSON(w, map[string]any{"jobId": id, "dpi": dpi})
}

// handleResolution returns the preview and output resolutions, the presets a
// picker offers, and the bounds a custom dpi stays inside, all resolved against
// the active template
func (s *server) handleResolution(w http.ResponseWriter, r *http.Request) {
	settings, err := s.resolutions()
	if err != nil {
		http.Error(w, "reading template manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, settings)
}

// resolutionBody is the POST /api/resolution payload. Both are dpi, and a zero
// output means the active template's own resolution
type resolutionBody struct {
	PreviewDPI int `json:"previewDpi"`
	OutputDPI  int `json:"outputDpi"`
}

// handleSetResolution stores the two resolutions and returns them resolved the
// way the GET does, so the client renders back what was actually kept rather
// than what it asked for
func (s *server) handleSetResolution(w http.ResponseWriter, r *http.Request) {
	var body resolutionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad resolution request: "+err.Error(), http.StatusBadRequest)
		return
	}
	m, err := s.pipe.manifest()
	if err != nil {
		http.Error(w, "reading template manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Clamping before the store keeps a dpi the active template cannot reach
	// from sitting in prefs and surprising a later, larger template
	s.prefs.setResolution(m.ClampDPI(previewOrDefault(body.PreviewDPI)), clampOutputDPI(m, body.OutputDPI))
	s.handleResolution(w, r)
}

// clampOutputDPI keeps a stored output resolution inside what the template can
// render, preserving zero as "the template's own" so the preference tracks a
// later template rather than pinning this one's number
func clampOutputDPI(m *template.Manifest, dpi int) int {
	if dpi <= 0 || dpi >= m.NativeDPI() {
		return 0
	}
	return m.ClampDPI(dpi)
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

// handleJobEvents streams a job from the job map. It serves render, template
// download and list resolve jobs
func (s *server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	j, ok := s.lookupJob(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	streamJob(w, r, j)
}

// streamJob writes a job's events as server-sent events, replaying the backlog
// to a late subscriber and ending after the terminal done event
func streamJob(w http.ResponseWriter, r *http.Request, j *job) {
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

// templateView is one template block: its name, its description, whether this
// build can render it, an optional reason, and its versions
type templateView struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Renderable  bool          `json:"renderable"`
	Reason      string        `json:"reason,omitempty"`
	Versions    []versionView `json:"versions"`
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
	rows := buildTemplateRows(idx, template.List(), func(n string) bool { return looseDir(n) != "" }, isVersionCached)

	views := make([]templateView, 0, len(rows))
	for _, row := range rows {
		tv := templateView{Name: row.name, Description: row.description, Renderable: row.renderable, Reason: row.reason}
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

// handleActiveTemplate returns the active template's name, version, the display
// label for the top-bar indicator, and where its assets come from for the status
// bar
func (s *server) handleActiveTemplate(w http.ResponseWriter, r *http.Request) {
	name, version := s.active()
	writeJSON(w, map[string]string{
		"name":    name,
		"version": version,
		"label":   templateDisplay(name, version),
		"source":  templateSource(name, version),
	})
}

// templateSource names where the active template's assets live: a cached
// bundle, a loose developer directory, or the placeholder layers
func templateSource(name, version string) string {
	switch {
	case version == "":
		return "placeholder"
	case version == localVersion:
		return "local"
	case isVersionCached(name, version):
		return "cached"
	default:
		return "bundle"
	}
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
	if s.runActive() {
		http.Error(w, "the template cannot change while a run is in progress", http.StatusConflict)
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
