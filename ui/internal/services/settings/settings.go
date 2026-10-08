// Package settings is the service behind the Settings panel: the stored
// interface settings, the render resolutions, what this computer can run and
// which features the build has switched on
package settings

import (
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/resource"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// The themes a settings write may carry. Anything else is stored as the default
var themes = map[string]bool{"dark": true, "light": true, "system": true}

// maxRecentOutputDirs bounds the recent output folders a settings write keeps
const maxRecentOutputDirs = 6

// concurrencyPresets are the worker counts the settings offer beside automatic,
// those that fit under the maximum
var concurrencyPresets = []int{1, 2, 4, 8, 12, 16}

// Service reads and writes the stored settings
type Service struct{ ws *workspace.Workspace }

// New returns the service over a workspace
func New(ws *workspace.Workspace) *Service { return &Service{ws: ws} }

// Capabilities returns the gate map the frontend applies to every feature-keyed
// control
func (s *Service) Capabilities() map[string]Gate { return capabilities() }

// Get returns the interface settings
func (s *Service) Get() prefs.Settings { return s.ws.Prefs.Settings() }

// Put replaces the interface settings and returns what was kept
func (s *Service) Put(in prefs.Settings) prefs.Settings {
	if !themes[in.Theme] {
		in.Theme = ""
	}
	if in.CardData != workspace.CardDataLocal {
		in.CardData = ""
	}
	if in.Concurrency < 0 || in.Concurrency > batch.MaxConcurrency {
		in.Concurrency = 0
	}
	if !resource.ValidCacheSize(in.LayerCache) {
		in.LayerCache = ""
	}
	if in.OutputFormat != "mpc" {
		in.OutputFormat = ""
	}
	if len(in.RecentOutputDirs) > maxRecentOutputDirs {
		in.RecentOutputDirs = in.RecentOutputDirs[:maxRecentOutputDirs]
	}
	s.ws.Prefs.SetSettings(in)
	pipeline.SetLayerCacheBytes(int64(resource.CacheBytes(s.ws.LayerCacheName())))
	return s.ws.Prefs.Settings()
}

// Resolutions is what Resolution returns: the two chosen resolutions, the
// presets a picker offers, and the bounds a custom dpi has to stay inside. Every
// field is resolved against the active template, so the frontend renders the
// numbers it is handed without knowing anything about the template's authored
// size
type Resolutions struct {
	Preview template.Resolution   `json:"preview"`
	Output  template.Resolution   `json:"output"`
	Presets []template.Resolution `json:"presets"`
	MinDPI  int                   `json:"minDpi"`
	MaxDPI  int                   `json:"maxDpi"`
}

// Resolution resolves the stored preview and output dpi against the active
// template. A template authored below a stored resolution clamps it rather than
// upscaling, so switching templates never leaves a preference that renders
// something the assets cannot support
func (s *Service) Resolution() (Resolutions, error) {
	m, err := s.ws.Pipe.Manifest()
	if err != nil {
		return Resolutions{}, apierr.Wrap(apierr.Internal, "reading template manifest", err)
	}
	preview, output := s.ws.Prefs.Resolution()
	return Resolutions{
		Preview: m.Resolution(workspace.PreviewOrDefault(preview)),
		Output:  m.Resolution(output),
		Presets: m.Presets(),
		MinDPI:  min(template.MinDPI, m.NativeDPI()),
		MaxDPI:  m.NativeDPI(),
	}, nil
}

// ResolutionRequest is the pair of resolutions to store. Both are dpi, and a
// zero output means the active template's own resolution
type ResolutionRequest struct {
	PreviewDPI int `json:"previewDpi"`
	OutputDPI  int `json:"outputDpi"`
}

// SetResolution stores the two resolutions and returns them resolved the way
// Resolution does, so the client renders back what was actually kept rather than
// what it asked for
func (s *Service) SetResolution(req ResolutionRequest) (Resolutions, error) {
	m, err := s.ws.Pipe.Manifest()
	if err != nil {
		return Resolutions{}, apierr.Wrap(apierr.Internal, "reading template manifest", err)
	}
	// Clamping before the store keeps a dpi the active template cannot reach
	// from sitting in prefs and surprising a later, larger template
	s.ws.Prefs.SetResolution(m.ClampDPI(workspace.PreviewOrDefault(req.PreviewDPI)), workspace.ClampOutputDPI(m, req.OutputDPI))
	return s.Resolution()
}

// ResourcesView is what Resources returns: what this computer has, what one
// render costs at the output resolution, and the counts the settings offer.
// BaseBytes already holds CacheBytes, the room for frame layers shared between
// cards. The page adds RenderBytes times a count to BaseBytes to show what that
// count is expected to use, so the estimate lives in one place
type ResourcesView struct {
	CPUs           int         `json:"cpus"`
	TotalBytes     uint64      `json:"totalBytes"`
	AvailableBytes uint64      `json:"availableBytes"`
	BudgetBytes    uint64      `json:"budgetBytes"`
	BaseBytes      uint64      `json:"baseBytes"`
	Cache          string      `json:"cache"`
	CacheBytes     uint64      `json:"cacheBytes"`
	CacheSizes     []CacheSize `json:"cacheSizes"`
	RenderBytes    uint64      `json:"renderBytes"`
	Auto           int         `json:"auto"`
	Max            int         `json:"max"`
	Presets        []int       `json:"presets"`
}

// CacheSize is one layer cache size the settings offer
type CacheSize struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Bytes uint64 `json:"bytes"`
}

// Resources returns this computer's memory and what a run is expected to use per
// render, so the concurrency setting can show real numbers. A non-empty cache
// names a layer cache size to plan for in place of the saved one, so the panel
// can show what a choice would do before it is saved
func (s *Service) Resources(cache string) ResourcesView {
	mem := workspace.ReadMemory()
	per := s.ws.RenderBytes()
	if !resource.ValidCacheSize(cache) {
		cache = s.ws.LayerCacheName()
	}
	cacheBytes := resource.CacheBytes(cache)
	v := ResourcesView{
		CPUs:           workspace.CPUCount(),
		TotalBytes:     mem.Total,
		AvailableBytes: mem.Available,
		BudgetBytes:    mem.Budget(),
		BaseBytes:      resource.BaseBytes(cacheBytes),
		Cache:          cache,
		CacheBytes:     cacheBytes,
		RenderBytes:    per,
		Auto:           resource.Workers(mem, per, workspace.CPUCount(), cacheBytes),
		Max:            batch.MaxConcurrency,
	}
	for _, c := range resource.CacheSizes {
		v.CacheSizes = append(v.CacheSizes, CacheSize{Name: c.Name, Label: c.Label, Bytes: c.Bytes})
	}
	for _, n := range concurrencyPresets {
		if n <= batch.MaxConcurrency {
			v.Presets = append(v.Presets, n)
		}
	}
	return v
}
