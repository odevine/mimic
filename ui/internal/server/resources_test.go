package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/pipeline"
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
	return getResourcesAt(t, s, "/api/resources")
}

func getResourcesAt(t *testing.T, s *Server, url string) resourcesView {
	t.Helper()
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
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
	if v.CacheBytes == 0 || v.BaseBytes < v.CacheBytes {
		t.Errorf("cache = %d, base = %d, want the base to hold the cache", v.CacheBytes, v.BaseBytes)
	}
	if v.RenderBytes == 0 || v.BaseBytes != resource.BaseBytes(v.CacheBytes) {
		t.Errorf("render = %d, base = %d", v.RenderBytes, v.BaseBytes)
	}
	if want := resource.Workers(resource.Memory{Total: 32 * gib, Available: 20 * gib}, v.RenderBytes, 12, v.CacheBytes); v.Auto != want {
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

func putSettings(t *testing.T, s *Server, body string) {
	t.Helper()
	// A test that picks a cache size must not leave it set for the next
	t.Cleanup(func() { pipeline.SetLayerCacheBytes(int64(resource.CacheBytes(""))) })
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResourcesOffersTheLayerCacheSizes(t *testing.T) {
	withComputer(t, resource.Memory{Total: 32 << 30, Available: 20 << 30}, 12)
	s := resolutionServer(t)

	v := getResources(t, s)
	if v.Cache != resource.DefaultCacheSize || v.CacheBytes != resource.CacheBytes("") {
		t.Errorf("cache = %q (%d bytes), want the default", v.Cache, v.CacheBytes)
	}
	if len(v.CacheSizes) != len(resource.CacheSizes) {
		t.Fatalf("%d sizes offered, want %d", len(v.CacheSizes), len(resource.CacheSizes))
	}
	for i, c := range v.CacheSizes {
		if want := resource.CacheSizes[i]; c.Name != want.Name || c.Label != want.Label || c.Bytes != want.Bytes {
			t.Errorf("size %d = %+v, want %+v", i, c, want)
		}
	}
}

func TestLayerCacheSettingChangesThePlan(t *testing.T) {
	withComputer(t, resource.Memory{Total: 16 << 30, Available: 10 << 30}, 16)
	s := resolutionServer(t)
	before := getResources(t, s)

	putSettings(t, s, `{"layerCache":"xlarge"}`)
	after := getResources(t, s)
	if after.Cache != "xlarge" || after.CacheBytes != resource.CacheBytes("xlarge") {
		t.Errorf("cache = %q (%d bytes), want xlarge", after.Cache, after.CacheBytes)
	}
	if got, want := after.BaseBytes-before.BaseBytes, resource.CacheBytes("xlarge")-before.CacheBytes; got != want {
		t.Errorf("the base grew by %d bytes, want %d", got, want)
	}
	if after.Auto > before.Auto {
		t.Errorf("a bigger cache raised auto from %d to %d", before.Auto, after.Auto)
	}
}

func TestResourcesPlansForACacheNotYetSaved(t *testing.T) {
	withComputer(t, resource.Memory{Total: 16 << 30, Available: 10 << 30}, 16)
	s := resolutionServer(t)
	saved := getResources(t, s)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/resources?cache=small", nil))
	var v resourcesView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Cache != "small" || v.CacheBytes != resource.CacheBytes("small") {
		t.Errorf("cache = %q (%d bytes), want small", v.Cache, v.CacheBytes)
	}
	if v.BaseBytes >= saved.BaseBytes {
		t.Errorf("a smaller cache left the base at %d, was %d", v.BaseBytes, saved.BaseBytes)
	}
	if got := s.prefs.Settings().LayerCache; got != "" {
		t.Errorf("asking about a size saved %q", got)
	}

	// A name that is not a size is ignored
	if got := getResourcesAt(t, s, "/api/resources?cache=huge"); got.Cache != saved.Cache {
		t.Errorf("an unknown cache name planned for %q, want the saved %q", got.Cache, saved.Cache)
	}
}

func TestSettingsKeepOnlyKnownLayerCacheSizes(t *testing.T) {
	s := runServer(t)
	for in, want := range map[string]string{"small": "small", "xlarge": "xlarge", "enormous": "", "": ""} {
		putSettings(t, s, `{"layerCache":"`+in+`"}`)
		if got := s.prefs.Settings().LayerCache; got != want {
			t.Errorf("layerCache %q stored as %q, want %q", in, got, want)
		}
	}
}
