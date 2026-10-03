// Package server is the local HTTP API behind the web frontend. It wires the
// catalog, card data, list resolution, rendering and batch runs to routes, and
// serves the frontend files beside them
package server

import (
	"context"
	"fmt"
	"image"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/carddata"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/scryfall"
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

// Server holds the shared, concurrency-safe pieces behind the HTTP API. The browser is the single driver of UI state, so
// the server keeps only what must be shared: the render pipeline, the art cache,
// persisted prefs, the in-flight jobs, and the active-template labels. Each is
// guarded by its own mutex or an atomic inside the pipeline
type Server struct {
	pipe  *pipeline.Pipeline
	prefs *prefs.Store
	mux   *http.ServeMux
	// static is the frontend served at the site root, and open shows a folder in
	// the system file browser
	static fs.FS
	open   func(path string) error

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
	recents   *prefs.Recents

	// resolveCancel abandons the list resolve in flight when a new one starts,
	// and resolved caches lookups for the session
	resolveMu     sync.Mutex
	resolveCancel context.CancelFunc
	resolved      cardlist.Cache

	// cardbackMu keeps two cardback uploads from mixing one's image with the
	// other's name
	cardbackMu sync.Mutex

	// run is the latest batch run, kept after it finishes until the next starts
	runMu  sync.Mutex
	run    *batch.Run
	runJob *job
	runSeq uint64

	// cards is the local copy of Scryfall bulk data, empty until downloaded
	cards *carddata.Store
	// scryfall is the paced HTTP client the card client uses, shared with the
	// bulk data download
	scryfall *http.Client
	// remoteCards caches Scryfall's bulk data index for the settings panel
	remoteCards remoteCache
}

// Options is what the server needs from the binary around it
type Options struct {
	// Static is the frontend served at the site root
	Static fs.FS
	// Open shows a path in the system file browser
	Open func(path string) error
}

// New builds the server: it resolves the startup template without
// blocking, installs it, and wires the routes. When the startup template came
// from a bundle or a placeholder it kicks off the background default-template
// auto-update, matching the desktop app
func New(o Options) *Server {
	httpc := scryfall.NewHTTPClient(netTimeout)
	p := loadPrefs()
	pipe := pipeline.New(card.NewClient(card.WithHTTPClient(httpc)), p.FaceTemplates)

	at, source := startupTemplate(p)
	pipe.Install(at)

	s := &Server{
		pipe:          pipe,
		prefs:         p,
		artCache:      make(map[string]image.Image),
		jobs:          make(map[string]*job),
		activeName:    at.Name,
		activeVersion: at.Version,
		recents:       prefs.NewRecents(p.RecentSearches()),
		cards:         carddata.New(cardDir()),
		scryfall:      httpc,
		static:        o.Static,
		open:          o.Open,
	}
	s.routes()
	go s.cards.Load()

	// Keep the default template up to date in the background so launch never
	// blocks on a large download. A loose developer directory and an explicitly
	// restored selection are the user's choice, so neither is auto-updated
	if source == pipeline.SourceBundle || source == pipeline.SourcePlaceholder {
		go s.updateTemplateBackground(at.Name)
	}
	return s
}

// userConfigDir locates the per-OS user config directory that prefs, the
// template cache and card data all sit under. It is a var so a test can
// redirect them to a temporary directory, since os.UserConfigDir does not honor
// an override on every OS
var userConfigDir = os.UserConfigDir

// init places the template cache under userConfigDir, read at call time so a
// test that redirects the config directory redirects the cache too
func init() {
	catalog.Dir = func() (string, error) {
		base, err := userConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "mimic", "templates"), nil
	}
}

// loadPrefs reads prefs.json beside the bundle cache under the per-OS user
// config directory. It goes through userConfigDir so a test can redirect it the
// same way the cache does, and a missing config directory just disables saving
func loadPrefs() *prefs.Store {
	base, err := userConfigDir()
	if err != nil {
		return prefs.Load("")
	}
	return prefs.Load(filepath.Join(base, "mimic", "prefs.json"))
}

// cardDir is where the local card data lives, beside the template cache, or ""
// when there is no config directory, in which case local card data is simply
// unavailable
func cardDir() string {
	base, err := userConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "mimic", "scryfall")
}

// startupTemplate builds the template to render at launch. It restores the
// persisted selection only when it needs no download (a loose dir or an
// already-cached bundle), so launch is never blocked, and otherwise falls back
// to the default network-free chain for normal
func startupTemplate(p *prefs.Store) (*pipeline.Template, pipeline.Source) {
	name, ver := p.Template()
	restorable := name != "" && ((ver == pipeline.LocalVersion && pipeline.LooseDir(name) != "") || catalog.IsCached(name, ver))
	if restorable {
		if at, err := pipeline.FromVersion(context.Background(), name, ver, nil); err == nil {
			return at, pipeline.SourceExplicit
		} else {
			log.Printf("mimic: restoring template %s %s failed: %v", name, ver, err)
		}
	}
	at, source, err := pipeline.Resolve("normal")
	if err != nil {
		log.Fatalf("mimic: resolving template: %v", err)
	}
	return at, source
}

// Handler is the server's routes behind the loopback host guard
func (s *Server) Handler() http.Handler { return guard(s.mux) }

// Close releases every template the server installed. Call it once at shutdown
func (s *Server) Close() { s.pipe.Close() }

// updateTemplateBackground downloads the latest compatible version of a template
// and swaps it in. It runs at startup, so any failure is logged and the server
// keeps rendering from whatever it resolved to. The browser picks up the new
// active template on its next poll, and its next render uses it
func (s *Server) updateTemplateBackground(name string) {
	at, err := pipeline.Latest(context.Background(), name)
	if err != nil {
		log.Printf("mimic: template update skipped: %v", err)
		return
	}
	s.setActiveTemplate(at)
}

// setActiveTemplate installs a new template, records it for the indicator, and
// persists the choice so the next launch restores it
func (s *Server) setActiveTemplate(at *pipeline.Template) {
	s.pipe.Install(at)
	s.activeMu.Lock()
	s.activeName = at.Name
	s.activeVersion = at.Version
	s.activeMu.Unlock()
	s.prefs.SetTemplate(at.Name, at.Version)
}

// rememberQuery records a successful search and persists the updated list, so
// the search box suggests it next time
func (s *Server) rememberQuery(query string) {
	s.recentsMu.Lock()
	s.recents.Add(query)
	list := s.recents.List()
	s.recentsMu.Unlock()
	s.prefs.SetRecentSearches(list)
}

// recentQueries returns the suggestion list, newest first
func (s *Server) recentQueries() []string {
	s.recentsMu.Lock()
	defer s.recentsMu.Unlock()
	return append([]string(nil), s.recents.List()...)
}

// active returns the current template name and version for the indicator
func (s *Server) active() (name, version string) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return s.activeName, s.activeVersion
}

// artFor returns the card's art crop, fetched once per URL and reused from the
// cache on later renders. A card with no artwork URL is not an error. A fetch
// failure returns a nil image and the error, so the render still proceeds and
// the caller can report art as unavailable
func (s *Server) artFor(ctx context.Context, d *card.Data) (image.Image, error) {
	if d.ArtworkURL == "" {
		return nil, nil
	}
	s.artMu.Lock()
	if img, ok := s.artCache[d.ArtworkURL]; ok {
		s.artMu.Unlock()
		return img, nil
	}
	s.artMu.Unlock()

	art, err := s.pipe.FetchArt(ctx, d)
	if err != nil {
		return nil, err
	}
	s.cacheArt(d.ArtworkURL, art)
	return art, nil
}

// cacheArt stores an art crop, evicting the oldest entry when the cache is full
func (s *Server) cacheArt(url string, img image.Image) {
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
func (s *Server) newJob() (string, *job) {
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
func (s *Server) lookupJob(id string) (*job, bool) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

// doRender fetches the face's art (once per URL) and renders that face through
// the template chosen for it at dpi, streaming progress to the job and ending with a done event
// carrying whether art was missing
func (s *Server) doRender(j *job, d *card.Data, face, dpi int) {
	artCtx, cancelArt := context.WithTimeout(context.Background(), netTimeout)
	art, artErr := s.artFor(artCtx, d.Face(face))
	cancelArt()

	// The render is local work on a bound that has nothing to do with the
	// network's, and a full-resolution export is the slowest thing the app does
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	img, err := s.pipe.Render(ctx, d, face, art, dpi, throttleRender(func(step string, frac float64) {
		j.emit(jobEvent{Step: step, Frac: frac})
	}))
	if err != nil {
		j.emit(jobEvent{Done: true, Err: err.Error()})
		return
	}
	j.setResult(img, pipeline.FaceName(d, face))
	j.emit(jobEvent{Done: true, ArtMissing: artErr != nil})
}

// doSelectTemplate switches the active template to (name, version), downloading
// first when the version is not cached and streaming download progress to the
// job. With install set it only downloads, and the active template stays
func (s *Server) doSelectTemplate(j *job, name, version string, install bool) {
	var progress func(done, total int64)
	if !catalog.IsCached(name, version) && version != pipeline.LocalVersion {
		progress = throttleBytes(func(step string, frac float64) {
			j.emit(jobEvent{Step: step, Frac: frac})
		})
	}
	if install {
		if version == pipeline.LocalVersion {
			j.emit(jobEvent{Done: true})
			return
		}
		if _, err := catalog.EnsureVersion(context.Background(), name, version, progress); err != nil {
			j.emit(jobEvent{Done: true, Err: err.Error()})
			return
		}
		j.emit(jobEvent{Done: true})
		return
	}
	at, err := pipeline.FromVersion(context.Background(), name, version, progress)
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
