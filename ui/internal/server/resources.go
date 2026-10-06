package server

import (
	"net/http"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/resource"
)

// readMemory is how the server reads this computer's memory, a variable so a
// test can stand in a computer of any size
var readMemory = resource.ReadMemory

// cpuCount is how many processors the server sizes a run for
var cpuCount = resource.CPUs

// fallbackWidth and fallbackHeight size a render when the template's manifest
// cannot be read, which is the authored size of the frames the app ships
const (
	fallbackWidth  = 3264
	fallbackHeight = 4440
)

// concurrencyPresets are the worker counts the settings offer beside automatic,
// those that fit under the maximum
var concurrencyPresets = []int{1, 2, 4, 8, 12, 16}

// renderBytes estimates the memory one render adds at the output resolution, the
// size a run renders at
func (s *Server) renderBytes() uint64 {
	m, err := s.pipe.Manifest()
	if err != nil {
		return resource.RenderBytes(fallbackWidth, fallbackHeight)
	}
	scaled := m.Scaled(m.ScaleForDPI(m.ClampDPI(s.renderDPI(targetOutput))))
	return resource.RenderBytes(scaled.Width, scaled.Height)
}

// layerCacheName is the layer cache size the settings choose, with the default
// standing in for an empty or unknown choice
func (s *Server) layerCacheName() string {
	if name := s.prefs.Settings().LayerCache; resource.ValidCacheSize(name) {
		return name
	}
	return resource.DefaultCacheSize
}

// autoConcurrency is how many cards a run renders at once when the setting is
// automatic, sized to this computer's memory and processors
func (s *Server) autoConcurrency() int {
	return resource.Workers(readMemory(), s.renderBytes(), cpuCount(), resource.CacheBytes(s.layerCacheName()))
}

// resourcesView is what the resources endpoint returns: what this computer has,
// what one render costs at the output resolution, and the counts the settings
// offer. BaseBytes already holds CacheBytes, the room for frame layers shared
// between cards. The browser adds RenderBytes times a count to BaseBytes to show
// what that count is expected to use, so the estimate lives in one place
type resourcesView struct {
	CPUs           int             `json:"cpus"`
	TotalBytes     uint64          `json:"totalBytes"`
	AvailableBytes uint64          `json:"availableBytes"`
	BudgetBytes    uint64          `json:"budgetBytes"`
	BaseBytes      uint64          `json:"baseBytes"`
	Cache          string          `json:"cache"`
	CacheBytes     uint64          `json:"cacheBytes"`
	CacheSizes     []cacheSizeView `json:"cacheSizes"`
	RenderBytes    uint64          `json:"renderBytes"`
	Auto           int             `json:"auto"`
	Max            int             `json:"max"`
	Presets        []int           `json:"presets"`
}

// cacheSizeView is one layer cache size the settings offer
type cacheSizeView struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Bytes uint64 `json:"bytes"`
}

// handleResources returns this computer's memory and what a run is expected to
// use per render, so the concurrency setting can show real numbers. A cache
// query parameter names a layer cache size to plan for in place of the saved
// one, so the panel can show what a choice would do before it is saved
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	mem := readMemory()
	per := s.renderBytes()
	cache := s.layerCacheName()
	if name := r.URL.Query().Get("cache"); resource.ValidCacheSize(name) {
		cache = name
	}
	cacheBytes := resource.CacheBytes(cache)
	v := resourcesView{
		CPUs:           cpuCount(),
		TotalBytes:     mem.Total,
		AvailableBytes: mem.Available,
		BudgetBytes:    mem.Budget(),
		BaseBytes:      resource.BaseBytes(cacheBytes),
		Cache:          cache,
		CacheBytes:     cacheBytes,
		RenderBytes:    per,
		Auto:           resource.Workers(mem, per, cpuCount(), cacheBytes),
		Max:            batch.MaxConcurrency,
	}
	for _, c := range resource.CacheSizes {
		v.CacheSizes = append(v.CacheSizes, cacheSizeView{Name: c.Name, Label: c.Label, Bytes: c.Bytes})
	}
	for _, n := range concurrencyPresets {
		if n <= batch.MaxConcurrency {
			v.Presets = append(v.Presets, n)
		}
	}
	writeJSON(w, v)
}
