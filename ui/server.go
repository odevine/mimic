package main

import (
	"context"
	"fmt"
	"image"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/odevine/mimic/engine/card"
)

// netTimeout bounds a search or an art fetch, matching the rendercard CLI
const netTimeout = 30 * time.Second

// renderTimeout bounds the compositing itself. A full-resolution export of a
// large template decodes and blends every layer at its authored size, so it is
// given far more room than a network round trip
const renderTimeout = 10 * time.Minute

// artCacheMax bounds the art cache so a long session does not grow without
// limit. Art crops are large, so a modest cap holds many recently viewed cards
// while keeping memory bounded
const artCacheMax = 24

// jobTTL is how long a finished job is kept so a slightly late client can still
// read its events and image before it is reaped
const jobTTL = 10 * time.Minute

// server holds the shared, concurrency-safe pieces behind the HTTP API. It
// replaces the Fyne ui struct. The browser is the single driver of UI state, so
// the server keeps only what must be shared: the render pipeline, the art cache,
// persisted prefs, the in-flight jobs, and the active-template labels. Each is
// guarded by its own mutex or an atomic inside the pipeline
type server struct {
	pipe  *renderPipeline
	prefs *prefs
	mux   *http.ServeMux

	// artCache reuses a card's fetched art across edits, keyed by ArtworkURL, so
	// any render whose card carries an already-seen URL skips the network. order
	// tracks insertion for a simple bounded FIFO eviction
	artMu    sync.Mutex
	artCache map[string]image.Image
	artOrder []string

	jobsMu sync.Mutex
	jobs   map[string]*job
	nextID uint64

	activeMu      sync.Mutex
	activeName    string
	activeVersion string

	// recents is the search-box suggestion list, seeded from prefs and persisted
	// back on every change. Guarded by its own mutex
	recentsMu sync.Mutex
	recents   *recents

	// resolveCancel abandons the list resolve in flight when a new one starts,
	// and resolved caches lookups for the session
	resolveMu     sync.Mutex
	resolveCancel context.CancelFunc
	resolved      resolveCache

	// run is the latest batch run, kept after it finishes until the next starts
	runMu  sync.Mutex
	run    *batchRun
	runSeq uint64

	// cards is the local copy of Scryfall bulk data, empty until downloaded
	cards *localStore
	// scryfall is the paced HTTP client the card client uses, shared with the
	// bulk data download
	scryfall *http.Client
	// remoteCards caches Scryfall's bulk data index for the settings panel
	remoteCards remoteCache
}

// newServer builds the server: it resolves the startup template without
// blocking, installs it, and wires the routes. When the startup template came
// from a bundle or a placeholder it kicks off the background default-template
// auto-update, matching the desktop app
func newServer() *server {
	httpc := scryfallHTTPClient()
	pipe := &renderPipeline{client: card.NewClient(card.WithHTTPClient(httpc))}
	p := loadPrefs()

	at, source := startupTemplate(p)
	pipe.install(at)

	s := &server{
		pipe:          pipe,
		prefs:         p,
		artCache:      make(map[string]image.Image),
		jobs:          make(map[string]*job),
		activeName:    at.name,
		activeVersion: at.version,
		recents:       newRecents(p.recentSearches()),
		cards:         newLocalStore(cardDir()),
		scryfall:      httpc,
	}
	s.routes()
	go s.cards.load()

	// Keep the default template up to date in the background so launch never
	// blocks on a large download. A loose developer directory and an explicitly
	// restored selection are the user's choice, so neither is auto-updated
	if source == sourceBundle || source == sourcePlaceholder {
		go s.updateTemplateBackground(at.name)
	}
	return s
}

// startupTemplate builds the template to render at launch. It restores the
// persisted selection only when it needs no download (a loose dir or an
// already-cached bundle), so launch is never blocked, and otherwise falls back
// to the default network-free chain for normal
func startupTemplate(p *prefs) (*activeTemplate, assetSource) {
	name, ver := p.template()
	restorable := name != "" && ((ver == localVersion && looseDir(name) != "") || isVersionCached(name, ver))
	if restorable {
		if at, err := activeFromVersion(context.Background(), name, ver, nil); err == nil {
			return at, sourceExplicit
		} else {
			log.Printf("mimic: restoring template %s %s failed: %v", name, ver, err)
		}
	}
	at, source, err := resolveActiveTemplate("normal")
	if err != nil {
		log.Fatalf("mimic: resolving template: %v", err)
	}
	return at, source
}

// updateTemplateBackground downloads the latest compatible version of a template
// and swaps it in. It runs at startup, so any failure is logged and the server
// keeps rendering from whatever it resolved to. The browser picks up the new
// active template on its next poll, and its next render uses it
func (s *server) updateTemplateBackground(name string) {
	at, err := autoLatestActive(context.Background(), name)
	if err != nil {
		log.Printf("mimic: template update skipped: %v", err)
		return
	}
	s.setActiveTemplate(at)
}

// setActiveTemplate installs a new template, records it for the indicator, and
// persists the choice so the next launch restores it
func (s *server) setActiveTemplate(at *activeTemplate) {
	s.pipe.install(at)
	s.activeMu.Lock()
	s.activeName = at.name
	s.activeVersion = at.version
	s.activeMu.Unlock()
	s.prefs.setTemplate(at.name, at.version)
}

// rememberQuery records a successful search and persists the updated list, so
// the search box suggests it next time
func (s *server) rememberQuery(query string) {
	s.recentsMu.Lock()
	s.recents.add(query)
	list := s.recents.list()
	s.recentsMu.Unlock()
	s.prefs.setRecentSearches(list)
}

// recentQueries returns the suggestion list, newest first
func (s *server) recentQueries() []string {
	s.recentsMu.Lock()
	defer s.recentsMu.Unlock()
	return append([]string(nil), s.recents.list()...)
}

// active returns the current template name and version for the indicator
func (s *server) active() (name, version string) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return s.activeName, s.activeVersion
}

// artFor returns the card's art crop, fetched once per URL and reused from the
// cache on later renders. A card with no artwork URL is not an error. A fetch
// failure returns a nil image and the error, so the render still proceeds and
// the caller can report art as unavailable
func (s *server) artFor(ctx context.Context, d *card.Data) (image.Image, error) {
	if d.ArtworkURL == "" {
		return nil, nil
	}
	s.artMu.Lock()
	if img, ok := s.artCache[d.ArtworkURL]; ok {
		s.artMu.Unlock()
		return img, nil
	}
	s.artMu.Unlock()

	art, err := s.pipe.fetchArt(ctx, d)
	if err != nil {
		return nil, err
	}
	s.cacheArt(d.ArtworkURL, art)
	return art, nil
}

// cacheArt stores an art crop, evicting the oldest entry when the cache is full
func (s *server) cacheArt(url string, img image.Image) {
	s.artMu.Lock()
	defer s.artMu.Unlock()
	if _, ok := s.artCache[url]; ok {
		return
	}
	if len(s.artOrder) >= artCacheMax {
		oldest := s.artOrder[0]
		s.artOrder = s.artOrder[1:]
		delete(s.artCache, oldest)
	}
	s.artCache[url] = img
	s.artOrder = append(s.artOrder, url)
}

// newJob creates and registers a job, reaping any finished jobs past their TTL
// so the jobs map does not grow unbounded
func (s *server) newJob() (string, *job) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	now := time.Now()
	for id, j := range s.jobs {
		j.mu.Lock()
		expired := j.finished && now.Sub(j.created) > jobTTL
		j.mu.Unlock()
		if expired {
			delete(s.jobs, id)
		}
	}
	s.nextID++
	id := strconv.FormatUint(s.nextID, 10)
	j := &job{created: now}
	s.jobs[id] = j
	return id, j
}

// lookupJob returns the job with the given id
func (s *server) lookupJob(id string) (*job, bool) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

// doRender fetches art (once per URL) and renders the card through the active
// template at dpi, streaming progress to the job and ending with a done event
// carrying whether art was missing
func (s *server) doRender(j *job, d *card.Data, dpi int) {
	artCtx, cancelArt := context.WithTimeout(context.Background(), netTimeout)
	art, artErr := s.artFor(artCtx, d)
	cancelArt()

	// The render is local work on a bound that has nothing to do with the
	// network's, and a full-resolution export is the slowest thing the app does
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	img, err := s.pipe.render(ctx, d, art, dpi, throttleRender(func(step string, frac float64) {
		j.emit(jobEvent{Step: step, Frac: frac})
	}))
	if err != nil {
		j.emit(jobEvent{Done: true, Err: err.Error()})
		return
	}
	j.setResult(img, d.Name)
	j.emit(jobEvent{Done: true, ArtMissing: artErr != nil})
}

// doSelectTemplate switches the active template to (name, version), downloading
// first when the version is not cached and streaming download progress to the
// job
func (s *server) doSelectTemplate(j *job, name, version string) {
	var progress func(done, total int64)
	if !isVersionCached(name, version) && version != localVersion {
		progress = throttleBytes(func(step string, frac float64) {
			j.emit(jobEvent{Step: step, Frac: frac})
		})
	}
	at, err := activeFromVersion(context.Background(), name, version, progress)
	if err != nil {
		j.emit(jobEvent{Done: true, Err: err.Error()})
		return
	}
	s.setActiveTemplate(at)
	j.emit(jobEvent{Done: true})
}

// throttleRender wraps a render progress callback with the same throttling the
// desktop app used: drop a repeat step whose fraction moved less than 0.01,
// except the final 1.0, so the SSE stream stays lean
func throttleRender(emit func(step string, frac float64)) func(step string, frac float64) {
	lastFrac := -1.0
	lastStep := ""
	return func(step string, frac float64) {
		if step == lastStep && frac-lastFrac < 0.01 && frac < 1 {
			return
		}
		lastStep, lastFrac = step, frac
		emit(step, frac)
	}
}

// throttleBytes adapts a byte-count download callback to a step/fraction emit,
// dropping moves smaller than 0.01 so a large bundle download does not flood the
// stream
func throttleBytes(emit func(step string, frac float64)) func(done, total int64) {
	last := -1.0
	return func(done, total int64) {
		if total <= 0 {
			return
		}
		frac := float64(done) / float64(total)
		if frac-last < 0.01 && frac < 1 {
			return
		}
		last = frac
		emit(fmt.Sprintf("Downloading %d%%", int(frac*100)), frac)
	}
}
