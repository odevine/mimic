// Package workspace holds the state the services share: the render pipeline, the
// persisted prefs, the art cache, the local card data, the fonts folder, the
// active template and the latest batch run. It guards that state and has no
// behavior of its own beyond that
package workspace

import (
	"context"
	"image"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/carddata"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/fontdir"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/resource"
	"github.com/odevine/mimic/ui/internal/scryfall"
)

// NetTimeout bounds a search or an art fetch, matching the rendercard CLI
const NetTimeout = 30 * time.Second

// RenderTimeout bounds the compositing itself. A full-resolution export of a
// large template decodes and blends every layer at its authored size, so it is
// given far more room than a network round trip
const RenderTimeout = 10 * time.Minute

// artCacheMax bounds the art cache so a long session does not grow without
// limit. Art crops are large, so a modest cap holds many recently viewed cards
// while keeping memory bounded
const artCacheMax = 24

// Card data sources, as the settings store them
const (
	CardDataAPI   = "api"
	CardDataLocal = "local"
)

// UserConfigDir locates the per-OS user config directory that prefs, the
// template cache and card data all sit under. It is a var so a test can
// redirect them to a temporary directory, since os.UserConfigDir does not honor
// an override on every OS
var UserConfigDir = os.UserConfigDir

// init places the template cache under UserConfigDir, read at call time so a
// test that redirects the config directory redirects the cache too
func init() {
	catalog.Dir = func() (string, error) {
		base, err := UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "mimic", "templates"), nil
	}
}

// Options is what the workspace needs from the binary around it
type Options struct {
	// FontDir is the -fonts flag, a font override directory that wins over
	// every other source. Empty means the usual lookup
	FontDir string
}

// Workspace is the shared, concurrency-safe state behind the services. Each
// piece is guarded by its own mutex or an atomic inside the pipeline
type Workspace struct {
	Pipe  *pipeline.Pipeline
	Prefs *prefs.Store
	Jobs  *jobs.Registry
	// Cards is the local copy of Scryfall bulk data, empty until downloaded
	Cards *carddata.Store
	// HTTP is the paced Scryfall client the card client uses, shared with the
	// bulk data download
	HTTP *http.Client
	// Fonts is the font override directory every render uses, fixed at startup
	Fonts fontdir.Dir
	// Run is the latest batch run
	Run RunState
	// Startup is the template the workspace began with and where it came from,
	// so the templates service can decide whether to update it in the background
	Startup StartupTemplate

	// artCache reuses a card's fetched art across edits, keyed by ArtworkURL, so
	// any render whose card carries an already-seen URL skips the network. order
	// tracks insertion for a simple bounded FIFO eviction
	artMu    sync.Mutex
	artCache map[string]image.Image
	artOrder []string

	activeMu      sync.Mutex
	activeName    string
	activeVersion string

	// recents is the search-box suggestion list, seeded from prefs and persisted
	// back on every change
	recentsMu sync.Mutex
	recents   *prefs.Recents
}

// StartupTemplate names the template a workspace began with and its origin
type StartupTemplate struct {
	Name   string
	Source pipeline.Source
}

// New builds the workspace: it resolves the startup template without blocking
// and installs it
func New(o Options) *Workspace {
	httpc := scryfall.NewHTTPClient(NetTimeout)
	p := LoadPrefs()
	pipeline.SetLayerCacheBytes(int64(resource.CacheBytes(p.Settings().LayerCache)))
	pipe := pipeline.New(card.NewClient(card.WithHTTPClient(httpc)), p.FaceTemplates)
	fd := fontdir.Resolve(o.FontDir, ManagedFontsDir())
	pipe.SetFontDir(fd.Path)

	at, source := startupTemplate(p)
	pipe.Install(at)

	return &Workspace{
		Pipe:          pipe,
		Prefs:         p,
		Jobs:          jobs.NewRegistry(),
		Cards:         carddata.New(CardDir()),
		HTTP:          httpc,
		Fonts:         fd,
		Startup:       StartupTemplate{Name: at.Name, Source: source},
		artCache:      make(map[string]image.Image),
		activeName:    at.Name,
		activeVersion: at.Version,
		recents:       prefs.NewRecents(p.RecentSearches()),
	}
}

// Close releases every template the workspace installed. Call it once at
// shutdown
func (w *Workspace) Close() { w.Pipe.Close() }

// LoadPrefs reads prefs.json beside the bundle cache under the per-OS user
// config directory. It goes through UserConfigDir so a test can redirect it the
// same way the cache does, and a missing config directory just disables saving
func LoadPrefs() *prefs.Store {
	base, err := UserConfigDir()
	if err != nil {
		return prefs.Load("")
	}
	return prefs.Load(filepath.Join(base, "mimic", "prefs.json"))
}

// CardDir is where the local card data lives, beside the template cache, or ""
// when there is no config directory, in which case local card data is simply
// unavailable
func CardDir() string {
	base, err := UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "mimic", "scryfall")
}

// ManagedFontsDir is the app's own fonts folder beside the template cache, or ""
// when there is no config directory
func ManagedFontsDir() string {
	base, err := UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "mimic", "fonts")
}

// CardbackPath is where the uploaded cardback is kept, beside prefs.json
func CardbackPath() (string, error) {
	base, err := UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "mimic", "cardback.png"), nil
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

// SetActiveTemplate installs a new template, records it for the indicator, and
// persists the choice so the next launch restores it
func (w *Workspace) SetActiveTemplate(at *pipeline.Template) {
	w.Pipe.Install(at)
	w.activeMu.Lock()
	w.activeName = at.Name
	w.activeVersion = at.Version
	w.activeMu.Unlock()
	w.Prefs.SetTemplate(at.Name, at.Version)
}

// Active returns the current template name and version for the indicator
func (w *Workspace) Active() (name, version string) {
	w.activeMu.Lock()
	defer w.activeMu.Unlock()
	return w.activeName, w.activeVersion
}

// RememberQuery records a successful search and persists the updated list, so
// the search box suggests it next time
func (w *Workspace) RememberQuery(query string) {
	w.recentsMu.Lock()
	w.recents.Add(query)
	list := w.recents.List()
	w.recentsMu.Unlock()
	w.Prefs.SetRecentSearches(list)
}

// RecentQueries returns the suggestion list, newest first
func (w *Workspace) RecentQueries() []string {
	w.recentsMu.Lock()
	defer w.recentsMu.Unlock()
	return append([]string(nil), w.recents.List()...)
}

// ArtFor returns the card's art crop, fetched once per URL and reused from the
// cache on later renders. A card with no artwork URL is not an error. A fetch
// failure returns a nil image and the error, so the render still proceeds and
// the caller can report art as unavailable
func (w *Workspace) ArtFor(ctx context.Context, d *card.Data) (image.Image, error) {
	if d.ArtworkURL == "" {
		return nil, nil
	}
	w.artMu.Lock()
	if img, ok := w.artCache[d.ArtworkURL]; ok {
		w.artMu.Unlock()
		return img, nil
	}
	w.artMu.Unlock()

	art, err := w.Pipe.FetchArt(ctx, d)
	if err != nil {
		return nil, err
	}
	w.cacheArt(d.ArtworkURL, art)
	return art, nil
}

// cacheArt stores an art crop, evicting the oldest entry when the cache is full
func (w *Workspace) cacheArt(url string, img image.Image) {
	w.artMu.Lock()
	defer w.artMu.Unlock()
	if w.artCache == nil {
		w.artCache = make(map[string]image.Image)
	}
	if _, ok := w.artCache[url]; ok {
		return
	}
	if len(w.artOrder) >= artCacheMax {
		oldest := w.artOrder[0]
		w.artOrder = w.artOrder[1:]
		delete(w.artCache, oldest)
	}
	w.artCache[url] = img
	w.artOrder = append(w.artOrder, url)
}

// UseLocalCards reports whether lookups should read the local copy of card data
func (w *Workspace) UseLocalCards() bool {
	return w.Cards != nil && w.Prefs.Settings().CardData == CardDataLocal && w.Cards.Ready()
}
