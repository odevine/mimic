package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/template"
)

// tinyBundle builds a minimal valid .mimic in memory and returns its bytes and
// SHA-256, mirroring the producer layout so download and the ZipAssetProvider
// both accept it
func tinyBundle(t *testing.T) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, method uint16, body string) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatalf("adding %q: %v", name, err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatalf("writing %q: %v", name, err)
		}
	}
	add("bundle.json", zip.Deflate, `{"format":1,"template":"normal","version":"0.1.0","minEngine":"0.3.0"}`)
	add("manifest.json", zip.Deflate, `{"template":"normal","width":744,"height":1039,"layers":[]}`)
	add("background/any.png", zip.Store, "fake-png-bytes")
	if err := zw.Close(); err != nil {
		t.Fatalf("closing bundle: %v", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

// serveCatalog stands up a server for a single template version and points the
// package's cache at a temp dir, returning the served version entry. Callers
// tweak the returned index before it is fetched by mutating what the handler
// closes over, so the SHA is set here to the real bundle hash
func serveCatalog(t *testing.T) (bundleSHA string, hits *int) {
	t.Helper()
	bundle, sha := tinyBundle(t)
	var downloads int

	mux := http.NewServeMux()
	mux.HandleFunc("/normal.mimic", func(w http.ResponseWriter, r *http.Request) {
		downloads++
		w.Write(bundle)
	})
	var srv *httptest.Server
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		idx := index{
			Schema: 1,
			Templates: []catalogTemplate{{
				Name:   "normal",
				Latest: "0.1.0",
				Versions: []catalogVersion{{
					Version:   "0.1.0",
					MinEngine: "0.3.0",
					URL:       srv.URL + "/normal.mimic",
					Size:      int64(len(bundle)),
					SHA256:    sha,
				}},
			}},
		}
		json.NewEncoder(w).Encode(idx)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tmp := t.TempDir()
	oldURL, oldCfg := indexURL, userConfigDir
	indexURL = srv.URL + "/index.json"
	userConfigDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { indexURL, userConfigDir = oldURL, oldCfg })

	return sha, &downloads
}

func TestEnsureTemplateDownloadsVerifiesAndCaches(t *testing.T) {
	_, downloads := serveCatalog(t)

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
	if *downloads != 1 {
		t.Errorf("downloads = %d, want 1", *downloads)
	}

	// A second call is served from the cache without re-downloading
	if _, err := ensureTemplate(context.Background(), "normal"); err != nil {
		t.Fatalf("second ensureTemplate: %v", err)
	}
	if *downloads != 1 {
		t.Errorf("downloads after cache hit = %d, want 1", *downloads)
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

func TestActiveFromVersionDownloadsAndBuilds(t *testing.T) {
	serveCatalog(t)

	var sawProgress bool
	progress := func(done, total int64) { sawProgress = true }

	at, err := activeFromVersion(context.Background(), "normal", "0.1.0", progress)
	if err != nil {
		t.Fatalf("activeFromVersion: %v", err)
	}
	defer at.cleanup()
	if at.name != "normal" || at.version != "0.1.0" {
		t.Errorf("active = %s %s, want normal 0.1.0", at.name, at.version)
	}
	if at.template == nil {
		t.Fatal("active template is nil")
	}
	if _, err := at.provider.Manifest(); err != nil {
		t.Fatalf("provider Manifest: %v", err)
	}
	if !sawProgress {
		t.Error("progress callback was never called during a download")
	}
	if !isVersionCached("normal", "0.1.0") {
		t.Error("version not cached after activeFromVersion")
	}
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	serveCatalog(t)

	bad := catalogVersion{Version: "0.1.0", MinEngine: "0.3.0", URL: indexServerBundleURL(t), SHA256: "deadbeef"}
	if _, err := download(context.Background(), "normal", bad, nil); err == nil {
		t.Fatal("expected a checksum mismatch error, got nil")
	}
	// A rejected download leaves nothing in the cache
	if versions, _ := cachedVersions("normal"); len(versions) != 0 {
		t.Errorf("cache holds %v after a mismatch, want empty", versions)
	}
}

// indexServerBundleURL fetches the catalog to recover the served bundle URL, so
// the mismatch test hits the same server without duplicating wiring
func indexServerBundleURL(t *testing.T) string {
	t.Helper()
	idx, err := fetchIndex(context.Background())
	if err != nil {
		t.Fatalf("fetchIndex: %v", err)
	}
	tmpl, ok := idx.findTemplate("normal")
	if !ok || len(tmpl.Versions) == 0 {
		t.Fatal("served catalog missing normal")
	}
	return tmpl.Versions[0].URL
}
