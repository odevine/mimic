package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/resource"
)

// withComputer stands in a computer with this memory and processor count for
// the length of a test
func withComputer(t *testing.T, mem resource.Memory, cpus int) {
	t.Helper()
	oldMem, oldCPU := readMemory, cpuCount
	readMemory = func() resource.Memory { return mem }
	cpuCount = func() int { return cpus }
	t.Cleanup(func() { readMemory, cpuCount = oldMem, oldCPU })
}

func getResources(t *testing.T, s *Server) resourcesView {
	t.Helper()
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/resources", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/resources = %d: %s", rec.Code, rec.Body.String())
	}
	var v resourcesView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	return v
}

func TestResourcesReportsTheComputerAndWhatARenderCosts(t *testing.T) {
	const gib = 1 << 30
	withComputer(t, resource.Memory{Total: 32 * gib, Available: 20 * gib}, 12)
	s := resolutionServer(t)

	v := getResources(t, s)
	if v.CPUs != 12 || v.TotalBytes != 32*gib || v.AvailableBytes != 20*gib {
		t.Errorf("computer = %d cpus, %d total, %d available", v.CPUs, v.TotalBytes, v.AvailableBytes)
	}
	if want := uint64(14 * gib); v.BudgetBytes != want {
		t.Errorf("budget = %d, want %d", v.BudgetBytes, want)
	}
	if v.RenderBytes == 0 || v.BaseBytes != resource.BaseBytes() {
		t.Errorf("render = %d, base = %d", v.RenderBytes, v.BaseBytes)
	}
	if want := resource.Workers(resource.Memory{Total: 32 * gib, Available: 20 * gib}, v.RenderBytes, 12); v.Auto != want {
		t.Errorf("auto = %d, want %d", v.Auto, want)
	}
	if v.Max != batch.MaxConcurrency {
		t.Errorf("max = %d, want %d", v.Max, batch.MaxConcurrency)
	}
	for _, n := range v.Presets {
		if n < 1 || n > v.Max {
			t.Errorf("preset %d is outside 1 to %d", n, v.Max)
		}
	}
	if len(v.Presets) == 0 || v.Presets[0] != 1 {
		t.Errorf("presets = %v, want them to start at one", v.Presets)
	}
}

func TestAutoConcurrencyFollowsMemory(t *testing.T) {
	const gib = 1 << 30
	s := resolutionServer(t)

	// A computer with almost no memory still renders, one card at a time
	withComputer(t, resource.Memory{Total: 1 * gib, Available: 128 << 20}, 16)
	if got := s.autoConcurrency(); got != 1 {
		t.Errorf("auto on a tiny computer = %d, want 1", got)
	}

	// Plenty of memory is limited by the processors
	withComputer(t, resource.Memory{Total: 512 * gib, Available: 400 * gib}, 6)
	if got := s.autoConcurrency(); got != 6 {
		t.Errorf("auto with six processors = %d, want 6", got)
	}
}
