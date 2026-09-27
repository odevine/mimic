package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/catalog/catalogtest"
)

// serveCatalog stands up a one-template catalog and points the package's index
// and cache at it and a temp dir
func serveCatalog(t *testing.T) *catalogtest.Server {
	t.Helper()
	srv := catalogtest.Serve(t)
	tmp := t.TempDir()
	oldURL, oldDir := IndexURL, Dir
	IndexURL = srv.IndexURL
	Dir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { IndexURL, Dir = oldURL, oldDir })
	return srv
}

func TestEnsureTemplateDownloadsVerifiesAndCaches(t *testing.T) {
	srv := serveCatalog(t)

	path, err := ensureTemplate(context.Background(), "normal")
	if err != nil {
		t.Fatalf("ensureTemplate: %v", err)
	}
	if filepath.Base(path) != "0.1.0.mimic" {
		t.Errorf("cached at %q, want a 0.1.0.mimic filename", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("bundle not in cache: %v", err)
	}
	if srv.Downloads() != 1 {
		t.Errorf("downloads = %d, want 1", srv.Downloads())
	}

	// A second call is served from the cache without re-downloading
	if _, err := ensureTemplate(context.Background(), "normal"); err != nil {
		t.Fatalf("second ensureTemplate: %v", err)
	}
	if srv.Downloads() != 1 {
		t.Errorf("downloads after cache hit = %d, want 1", srv.Downloads())
	}

	// The cached file opens as a valid bundle through the real provider
	cached, ok := newestCachedBundle("normal")
	if !ok {
		t.Fatal("newestCachedBundle found nothing after a download")
	}
	zp, err := template.NewZipAssetProvider(cached)
	if err != nil {
		t.Fatalf("opening cached bundle: %v", err)
	}
	zp.Close()
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	srv := serveCatalog(t)

	bad := Version{Version: "0.1.0", MinEngine: "0.3.0", URL: srv.BundleURL, SHA256: "deadbeef"}
	if _, err := download(context.Background(), "normal", bad, nil); err == nil {
		t.Fatal("expected a checksum mismatch error, got nil")
	}
	// A rejected download leaves nothing in the cache
	if versions, _ := CachedVersions("normal"); len(versions) != 0 {
		t.Errorf("cache holds %v after a mismatch, want empty", versions)
	}
}

func TestFetchIndexFallsBackToCache(t *testing.T) {
	srv := serveCatalog(t)
	if _, err := FetchIndex(context.Background()); err != nil {
		t.Fatalf("FetchIndex: %v", err)
	}

	// With the network gone, the copy cached by the first fetch still answers
	IndexURL = srv.IndexURL + ".missing"
	idx, err := FetchIndex(context.Background())
	if err != nil {
		t.Fatalf("FetchIndex offline: %v", err)
	}
	if tmpl, ok := idx.Find("normal"); !ok || len(tmpl.Versions) == 0 || tmpl.Versions[0].URL != srv.BundleURL {
		t.Errorf("cached index = %+v, want normal served from %s", idx, srv.BundleURL)
	}
}
