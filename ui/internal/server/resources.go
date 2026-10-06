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

// autoConcurrency is how many cards a run renders at once when the setting is
// automatic, sized to this computer's memory and processors
func (s *Server) autoConcurrency() int {
	return resource.Workers(readMemory(), s.renderBytes(), cpuCount())
}

// resourcesView is what the resources endpoint returns: what this computer has,
// what one render costs at the output resolution, and the counts the settings
// offer. The browser multiplies BaseBytes plus RenderBytes by a count to show
// what that count is expected to use, so the estimate lives in one place
type resourcesView struct {
	CPUs           int    `json:"cpus"`
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	BudgetBytes    uint64 `json:"budgetBytes"`
	BaseBytes      uint64 `json:"baseBytes"`
	RenderBytes    uint64 `json:"renderBytes"`
	Auto           int    `json:"auto"`
	Max            int    `json:"max"`
	Presets        []int  `json:"presets"`
}

// handleResources returns this computer's memory and what a run is expected to
// use per render, so the concurrency setting can show real numbers
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	mem := readMemory()
	per := s.renderBytes()
	v := resourcesView{
		CPUs:           cpuCount(),
		TotalBytes:     mem.Total,
		AvailableBytes: mem.Available,
		BudgetBytes:    mem.Budget(),
		BaseBytes:      resource.BaseBytes(),
		RenderBytes:    per,
		Auto:           resource.Workers(mem, per, cpuCount()),
		Max:            batch.MaxConcurrency,
	}
	for _, n := range concurrencyPresets {
		if n <= batch.MaxConcurrency {
			v.Presets = append(v.Presets, n)
		}
	}
	writeJSON(w, v)
}
