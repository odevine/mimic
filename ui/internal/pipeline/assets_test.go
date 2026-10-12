package pipeline

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/catalog/catalogtest"
	"github.com/odevine/mimic/ui/internal/templatetest"
)

// withEmptyAssetChain points the loose-dir roots at nothing and the cache at a
// fresh temp dir, so a test drives the fallback tiers deterministically
func withEmptyAssetChain(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	oldBases, oldDir := LooseDirBases, catalog.Dir
	LooseDirBases = []string{filepath.Join(tmp, "no-such-assets")}
	catalog.Dir = func() (string, error) { return filepath.Join(tmp, "templates"), nil }
	t.Cleanup(func() { LooseDirBases, catalog.Dir = oldBases, oldDir })
	return tmp
}

func TestResolveFallsBackToThePlaceholder(t *testing.T) {
	withEmptyAssetChain(t)

	at, source, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer at.Close()
	if source != SourcePlaceholder {
		t.Fatalf("source = %d, want SourcePlaceholder", source)
	}
	if at.Name != render.PlaceholderID || at.Version != "" {
		t.Errorf("template = %s %q, want the placeholder", at.Name, at.Version)
	}
	if !at.Serves(PrimaryShape) {
		t.Error("the placeholder does not render standard cards")
	}
}

func seedBundle(t *testing.T, spec templatetest.Spec) {
	t.Helper()
	bundle, _ := spec.Bundle(t)
	dst, err := catalog.BundlePath(spec.ID, spec.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, bundle, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePrefersCachedBundle(t *testing.T) {
	withEmptyAssetChain(t)
	// Seed the cache with a valid bundle so the middle tier wins
	seedBundle(t, templatetest.Spec{ID: catalogtest.ID, Version: "0.1.0"})

	at, source, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer at.Close()
	if source != SourceBundle {
		t.Fatalf("source = %d, want SourceBundle", source)
	}
	if at.Name != catalogtest.ID || at.Version != "0.1.0" {
		t.Errorf("template = %s %s, want %s 0.1.0", at.Name, at.Version, catalogtest.ID)
	}
}

// TestResolvePrefersLooseDir confirms a loose developer directory wins the chain,
// found by the id in its manifest and not by the name of its folder
func TestResolvePrefersLooseDir(t *testing.T) {
	tmp := withEmptyAssetChain(t)
	LooseDirBases = []string{filepath.Join(tmp, "assets")}
	spec := templatetest.Spec{ID: catalogtest.ID}
	spec.WriteLoose(t, filepath.Join(tmp, "assets", "frame"))
	seedBundle(t, spec)

	if got := LooseDir(catalogtest.ID); got != filepath.Join(tmp, "assets", "frame") {
		t.Errorf("LooseDir = %q", got)
	}
	at, source, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer at.Close()
	if source != SourceLoose {
		t.Fatalf("source = %d, want SourceLoose", source)
	}
	if at.Version != LocalVersion {
		t.Errorf("version = %q, want %q", at.Version, LocalVersion)
	}
	if _, ok := at.provider.(*template.CachedAssets); !ok {
		t.Errorf("provider is a %T, want the layer cache", at.provider)
	}
}

func TestInstalledTemplatesAreTheLooseFoldersAndCachedBundles(t *testing.T) {
	tmp := withEmptyAssetChain(t)
	LooseDirBases = []string{filepath.Join(tmp, "assets")}
	templatetest.Spec{ID: "test.loose"}.WriteLoose(t, filepath.Join(tmp, "assets", "loose"))
	seedBundle(t, templatetest.Spec{ID: "test.cached", Version: "0.2.0", Shapes: templatetest.Transform})
	// Left over from the earlier format: a folder in the cache that is not an id
	old, _ := catalog.BundlePath("normal", "0.3.0")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("not a bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	// And a folder that is not a template
	if err := os.MkdirAll(filepath.Join(tmp, "assets", "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := InstalledTemplates()
	if len(got) != 2 || got[0].ID != "test.cached" || got[1].ID != "test.loose" {
		t.Fatalf("installed = %+v, want test.cached and test.loose", got)
	}
	if len(got[0].Shapes) != 2 || !slices.ContainsFunc(got[0].Shapes, func(sh template.Shape) bool { return sh == templatetest.Transform[0] }) {
		t.Errorf("shapes of the cached template = %v", got[0].Shapes)
	}
	if v := InstalledVersions("test.loose"); !slices.Equal(v, []string{LocalVersion}) {
		t.Errorf("versions of the loose template = %v", v)
	}
	if v := InstalledVersions("test.cached"); !slices.Equal(v, []string{"0.2.0"}) {
		t.Errorf("versions of the cached template = %v", v)
	}
}

func TestResolveRefusesAFolderThatIsAnotherTemplate(t *testing.T) {
	tmp := withEmptyAssetChain(t)
	seedBundle(t, templatetest.Spec{ID: "test.other", Version: "0.1.0"})
	// A bundle moved into another template's folder is not that template
	moved, _ := catalog.BundlePath("test.other", "0.1.0")
	dst, _ := catalog.BundlePath("test.frame", "0.1.0")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, dst); err != nil {
		t.Fatal(err)
	}
	_ = tmp
	if at, err := activeFromCachedBundle("test.frame", "0.1.0"); err == nil {
		at.Close()
		t.Error("opened a bundle of another template as test.frame")
	}
}

func TestActiveFromVersionDownloadsAndBuilds(t *testing.T) {
	withEmptyAssetChain(t)
	srv := catalogtest.Serve(t)
	oldURL := catalog.IndexURL
	catalog.IndexURL = srv.IndexURL
	t.Cleanup(func() { catalog.IndexURL = oldURL })

	var sawProgress bool
	progress := func(done, total int64) { sawProgress = true }

	at, err := FromVersion(context.Background(), catalogtest.ID, "0.1.0", progress)
	if err != nil {
		t.Fatalf("FromVersion: %v", err)
	}
	defer at.Close()
	if at.Name != catalogtest.ID || at.Version != "0.1.0" {
		t.Errorf("active = %s %s, want %s 0.1.0", at.Name, at.Version, catalogtest.ID)
	}
	if at.tmpl == nil || !at.Serves(PrimaryShape) {
		t.Fatal("the template does not render standard cards")
	}
	if !sawProgress {
		t.Error("progress callback was never called during a download")
	}
	if !catalog.IsCached(catalogtest.ID, "0.1.0") {
		t.Error("version not cached after FromVersion")
	}
}

// loadAnyLayer decodes the first layer asset a template has through its provider
func loadAnyLayer(t *testing.T, at *Template) {
	t.Helper()
	for _, name := range slices.Sorted(maps.Keys(at.tmpl.Manifest().Parts)) {
		for _, key := range slices.Sorted(maps.Keys(at.tmpl.Manifest().Parts[name].Variants)) {
			if _, err := template.LoadImage(at.provider, at.tmpl.Manifest().Parts[name].Variants[key].Path); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("the manifest has no images")
}

func layerStats(at *Template) template.CacheStats {
	return at.provider.(*template.CachedAssets).Stats()
}

// resetLayerCache puts the budget back for the next test
func resetLayerCache(t *testing.T) {
	t.Cleanup(func() { SetLayerCacheBytes(template.DefaultImageCacheBytes) })
}

func TestSetLayerCacheBytesResizesLoadedTemplates(t *testing.T) {
	withEmptyAssetChain(t)
	resetLayerCache(t)
	at, _, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer at.Close()

	loadAnyLayer(t, at)
	if layerStats(at).Layers != 1 {
		t.Fatalf("stats = %+v, want the layer cached", layerStats(at))
	}
	SetLayerCacheBytes(0)
	if s := layerStats(at); s.Layers != 0 || s.Bytes != 0 {
		t.Errorf("stats after a zero budget = %+v, want empty", s)
	}
	loadAnyLayer(t, at)
	if layerStats(at).Layers != 0 {
		t.Error("a zero budget should keep nothing")
	}
	SetLayerCacheBytes(template.DefaultImageCacheBytes)
	loadAnyLayer(t, at)
	if layerStats(at).Layers != 1 {
		t.Error("raising the budget should cache again")
	}
}

func TestNewTemplatesStartWithTheChosenBudget(t *testing.T) {
	withEmptyAssetChain(t)
	resetLayerCache(t)
	SetLayerCacheBytes(0)
	at, _, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer at.Close()
	loadAnyLayer(t, at)
	if layerStats(at).Layers != 0 {
		t.Error("a template loaded under a zero budget should cache nothing")
	}
}

func TestInstallEmptiesTheSwappedOutLayerCache(t *testing.T) {
	withEmptyAssetChain(t)
	resetLayerCache(t)
	first, _, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := New(nil, nil)
	defer p.Close()
	p.Install(first)
	loadAnyLayer(t, first)
	if layerStats(first).Layers != 1 {
		t.Fatalf("stats = %+v, want the layer cached", layerStats(first))
	}

	p.Install(second)
	if layerStats(first).Layers != 0 {
		t.Error("the swapped-out template kept its layers")
	}
	// Later size changes must not bring the dead cache back
	SetLayerCacheBytes(template.DefaultImageCacheBytes)
	loadAnyLayer(t, first)
	if layerStats(first).Layers != 0 {
		t.Error("a resize revived the swapped-out template's cache")
	}
	p.Install(second)
	loadAnyLayer(t, second)
	if layerStats(second).Layers != 1 {
		t.Error("reinstalling the active template should leave its cache alone")
	}
}

func TestAChangedLooseFolderIsLoadedAgain(t *testing.T) {
	tmp := withEmptyAssetChain(t)
	LooseDirBases = []string{filepath.Join(tmp, "assets")}
	dir := filepath.Join(tmp, "assets", "frame")
	templatetest.Spec{ID: catalogtest.ID, Name: "First"}.WriteLoose(t, dir)

	at, _, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := New(nil, nil)
	defer p.Close()
	p.Install(at)
	if p.Active() != at || at.Info().Name != "First" {
		t.Fatalf("installed %q", at.Info().Name)
	}

	// Nothing changed, so nothing is loaded again
	p.checked = time.Time{}
	p.reloadChanged()
	if p.Active() != at {
		t.Error("an unchanged folder was loaded again")
	}

	templatetest.Spec{ID: catalogtest.ID, Name: "Second"}.WriteLoose(t, dir)
	p.checked = time.Time{}
	p.reloadChanged()
	if p.Active() == at || p.Active().Info().Name != "Second" {
		t.Errorf("the edited folder was not loaded again: %q", p.Active().Info().Name)
	}
	if p.Active().Name != catalogtest.ID || p.Active().Version != LocalVersion {
		t.Errorf("reloaded as %s %s", p.Active().Name, p.Active().Version)
	}
}

func TestARenderReadsTheFeaturesChosenForItsTemplate(t *testing.T) {
	tmp := withEmptyAssetChain(t)
	LooseDirBases = []string{filepath.Join(tmp, "assets")}
	templatetest.Spec{ID: catalogtest.ID, Features: templatetest.Features}.WriteLoose(t, filepath.Join(tmp, "assets", "frame"))
	at, _, err := Resolve(catalogtest.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := New(nil, nil)
	defer p.Close()
	p.Install(at)

	chosen := map[string]any{}
	p.SetFeatures(func(id string) map[string]any {
		if id != catalogtest.ID {
			t.Errorf("features asked for %q", id)
		}
		return chosen
	})
	d := &card.Data{Name: "Plain", TypeLine: "Instant", Layout: "normal"}
	stripe := func() bool {
		img, err := p.Render(context.Background(), d, 0, nil, 72, nil)
		if err != nil {
			t.Fatal(err)
		}
		// The banner is the crown's gold, darker than any background
		r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()*60/1039).RGBA()
		return r>>8 == 0xA0 && g>>8 == 0x7E && b>>8 == 0x2A
	}
	if stripe() {
		t.Error("the banner is drawn before anyone chose it")
	}
	chosen["banner"] = true
	if !stripe() {
		t.Error("the banner is not drawn once it is on")
	}
}
