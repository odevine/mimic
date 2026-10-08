package settings

import (
	"testing"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/resource"
	"github.com/odevine/mimic/ui/internal/workspace"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

func TestPutSanitizesTheme(t *testing.T) {
	workspacetest.WithEmptyAssetChain(t)
	svc := New(&workspace.Workspace{Prefs: workspace.LoadPrefs()})
	svc.Put(prefs.Settings{Theme: "neon", ExpandPrintings: true, Splits: map[string][]float64{"single": {300, 420}}})

	got := workspace.LoadPrefs().Settings()
	if got.Theme != "" {
		t.Errorf("theme %q persisted, want an unknown theme dropped", got.Theme)
	}
	if !got.ExpandPrintings || len(got.Splits["single"]) != 2 {
		t.Errorf("settings not persisted: %+v", got)
	}
}

func TestResolutionsDefault(t *testing.T) {
	ws := workspacetest.New(t)
	got, err := New(ws).Resolution()
	if err != nil {
		t.Fatalf("resolution: %v", err)
	}
	native := workspacetest.NativeDPI(t, ws)

	// Nothing has been chosen, so the preview takes the default and the output
	// takes the template's own resolution
	if want := min(workspace.DefaultPreviewDPI, native); got.Preview.DPI != want {
		t.Errorf("preview dpi = %d, want %d", got.Preview.DPI, want)
	}
	if got.Output.DPI != native || !got.Output.Native {
		t.Errorf("output = %+v, want the native %d dpi", got.Output, native)
	}
	if got.MaxDPI != native {
		t.Errorf("maxDpi = %d, want %d", got.MaxDPI, native)
	}
	if len(got.Presets) == 0 {
		t.Fatal("no presets offered")
	}
	if last := got.Presets[len(got.Presets)-1]; !last.Native {
		t.Errorf("last preset = %+v, want the native one", last)
	}
}

func TestRenderDPIByTarget(t *testing.T) {
	ws := workspacetest.New(t)
	native := workspacetest.NativeDPI(t, ws)
	ws.Prefs.SetResolution(96, 0)

	if got := ws.RenderDPI(workspace.TargetPreview); got != 96 {
		t.Errorf("preview dpi = %d, want 96", got)
	}
	// Zero output means the template's own resolution, which the engine
	// resolves from the manifest rather than the preference
	if got := ws.RenderDPI(workspace.TargetOutput); got != 0 {
		t.Errorf("output dpi = %d, want 0 for the template's own", got)
	}
	if got := ws.ClampedDPI(workspace.TargetOutput); got != native {
		t.Errorf("clamped output dpi = %d, want the native %d", got, native)
	}
}

func TestSetResolutionClampsAndPersists(t *testing.T) {
	ws := workspacetest.New(t)
	native := workspacetest.NativeDPI(t, ws)

	got, err := New(ws).SetResolution(ResolutionRequest{PreviewDPI: 1, OutputDPI: 999999})
	if err != nil {
		t.Fatal(err)
	}
	// A dpi under the floor clamps up, and one past the template's own clamps
	// back to it
	if want := min(template.MinDPI, native); got.Preview.DPI != want {
		t.Errorf("preview dpi = %d, want %d", got.Preview.DPI, want)
	}
	if got.Output.DPI != native {
		t.Errorf("output dpi = %d, want the native %d", got.Output.DPI, native)
	}

	// An output at or past native is stored as zero, so it follows a later
	// template rather than pinning this one's number
	if _, output := ws.Prefs.Resolution(); output != 0 {
		t.Errorf("stored output dpi = %d, want 0", output)
	}
}

func TestResolutionRoundTrip(t *testing.T) {
	ws := workspacetest.New(t)
	svc := New(ws)
	native := workspacetest.NativeDPI(t, ws)
	if native <= template.MinDPI*2 {
		t.Skipf("template native dpi %d leaves no room for an in-range choice", native)
	}
	pick := native / 2

	if _, err := svc.SetResolution(ResolutionRequest{PreviewDPI: pick, OutputDPI: pick}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Resolution()
	if err != nil {
		t.Fatal(err)
	}
	if got.Preview.DPI != pick || got.Output.DPI != pick {
		t.Errorf("round trip = %d/%d, want %d/%d", got.Preview.DPI, got.Output.DPI, pick, pick)
	}
	if got.Preview.Width <= 0 || got.Preview.Height <= 0 {
		t.Errorf("preview size = %dx%d, want a positive size", got.Preview.Width, got.Preview.Height)
	}
	if got.Output.Native {
		t.Error("an output below native was reported as native")
	}
}

// withComputer stands in a computer with this memory and processor count for
// the length of a test
func withComputer(t *testing.T, mem resource.Memory, cpus int) {
	t.Helper()
	oldMem, oldCPU := workspace.ReadMemory, workspace.CPUCount
	workspace.ReadMemory = func() resource.Memory { return mem }
	workspace.CPUCount = func() int { return cpus }
	t.Cleanup(func() { workspace.ReadMemory, workspace.CPUCount = oldMem, oldCPU })
}

func TestResourcesReportsTheComputerAndWhatARenderCosts(t *testing.T) {
	const gib = 1 << 30
	withComputer(t, resource.Memory{Total: 32 * gib, Available: 20 * gib}, 12)
	v := New(workspacetest.New(t)).Resources("")

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
	ws := workspacetest.New(t)

	// A computer with almost no memory still renders, one card at a time
	withComputer(t, resource.Memory{Total: 1 * gib, Available: 128 << 20}, 16)
	if got := ws.AutoConcurrency(); got != 1 {
		t.Errorf("auto on a tiny computer = %d, want 1", got)
	}

	// Plenty of memory is limited by the processors
	withComputer(t, resource.Memory{Total: 512 * gib, Available: 400 * gib}, 6)
	if got := ws.AutoConcurrency(); got != 6 {
		t.Errorf("auto with six processors = %d, want 6", got)
	}
}

// putLayerCache stores a layer cache size, and restores the default after the
// test so the next one does not inherit it
func putLayerCache(t *testing.T, svc *Service, size string) {
	t.Helper()
	t.Cleanup(func() { pipeline.SetLayerCacheBytes(int64(resource.CacheBytes(""))) })
	svc.Put(prefs.Settings{LayerCache: size})
}

func TestResourcesOffersTheLayerCacheSizes(t *testing.T) {
	withComputer(t, resource.Memory{Total: 32 << 30, Available: 20 << 30}, 12)
	v := New(workspacetest.New(t)).Resources("")

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
	svc := New(workspacetest.New(t))
	before := svc.Resources("")

	putLayerCache(t, svc, "xlarge")
	after := svc.Resources("")
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
	ws := workspacetest.New(t)
	svc := New(ws)
	saved := svc.Resources("")

	v := svc.Resources("small")
	if v.Cache != "small" || v.CacheBytes != resource.CacheBytes("small") {
		t.Errorf("cache = %q (%d bytes), want small", v.Cache, v.CacheBytes)
	}
	if v.BaseBytes >= saved.BaseBytes {
		t.Errorf("a smaller cache left the base at %d, was %d", v.BaseBytes, saved.BaseBytes)
	}
	if got := ws.Prefs.Settings().LayerCache; got != "" {
		t.Errorf("asking about a size saved %q", got)
	}

	// A name that is not a size is ignored
	if got := svc.Resources("huge"); got.Cache != saved.Cache {
		t.Errorf("an unknown cache name planned for %q, want the saved %q", got.Cache, saved.Cache)
	}
}

func TestSettingsKeepOnlyKnownLayerCacheSizes(t *testing.T) {
	ws := workspacetest.New(t)
	svc := New(ws)
	for in, want := range map[string]string{"small": "small", "xlarge": "xlarge", "enormous": "", "": ""} {
		putLayerCache(t, svc, in)
		if got := ws.Prefs.Settings().LayerCache; got != want {
			t.Errorf("layerCache %q stored as %q, want %q", in, got, want)
		}
	}
}

func TestSettingsKeepOnlyKnownOutputFormats(t *testing.T) {
	ws := workspacetest.New(t)
	svc := New(ws)
	for in, want := range map[string]string{"mpc": "mpc", "pdf": "", "": ""} {
		svc.Put(prefs.Settings{OutputFormat: in})
		if got := ws.Prefs.Settings().OutputFormat; got != want {
			t.Errorf("outputFormat %q stored as %q, want %q", in, got, want)
		}
	}
}
