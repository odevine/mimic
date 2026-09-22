package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// indexURL is the catalog the app fetches to discover templates and versions.
// It is served from the templates repo's default branch, a stable raw URL that
// can later be swapped for a CDN-backed one without any code change here. It is
// a var so a test can point it at a local server
var indexURL = "https://raw.githubusercontent.com/odevine/mimic-templates/main/index.json"

// userConfigDir locates the per-OS user config directory. It is a var so a test
// can redirect the cache to a temporary directory, since os.UserConfigDir does
// not honor an override on every OS
var userConfigDir = os.UserConfigDir

// indexFetchTimeout bounds the catalog fetch. index.json is tiny, so a short
// timeout is enough and a slow network falls back to the cached copy quickly
const indexFetchTimeout = 15 * time.Second

// index is the whole catalog: every template and its downloadable versions. It
// mirrors the schema tools/reindex writes in the templates repo
type index struct {
	Schema    int               `json:"schema"`
	Templates []catalogTemplate `json:"templates"`
}

// catalogTemplate groups every published version of one template. Versions are
// sorted newest first, so Versions[0] is Latest
type catalogTemplate struct {
	Name     string           `json:"name"`
	Latest   string           `json:"latest"`
	Versions []catalogVersion `json:"versions"`
}

// catalogVersion is one downloadable bundle. SHA256 gates the download's
// integrity; MinEngine lets the app filter incompatible versions before
// fetching anything
type catalogVersion struct {
	Version   string `json:"version"`
	MinEngine string `json:"minEngine"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

// cacheDir is where downloaded bundles and the last-fetched catalog live, under
// the per-OS user config directory so it is correct on macOS, Linux, and Windows
func cacheDir() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "mimic", "templates"), nil
}

// fetchIndex returns the catalog, preferring a fresh copy from the network and
// falling back to the last one cached so the app works offline against what it
// already has. A successful fetch refreshes the cache
func fetchIndex(ctx context.Context) (*index, error) {
	dir, err := cacheDir()
	if err != nil {
		return nil, err
	}
	cachePath := filepath.Join(dir, "index.json")

	fetchCtx, cancel := context.WithTimeout(ctx, indexFetchTimeout)
	defer cancel()
	body, err := getBytes(fetchCtx, indexURL)
	if err != nil {
		return cachedIndex(cachePath, err)
	}
	var idx index
	if err := json.Unmarshal(body, &idx); err != nil {
		return cachedIndex(cachePath, fmt.Errorf("decoding index: %w", err))
	}
	// Refresh the cache best effort; a write failure does not fail the fetch
	if err := os.MkdirAll(dir, 0o755); err == nil {
		_ = os.WriteFile(cachePath, body, 0o644)
	}
	return &idx, nil
}

// cachedIndex reads the last-fetched catalog, used when the network fetch fails.
// It wraps the original fetch error when no cache exists so the caller sees why
// the live fetch failed
func cachedIndex(path string, fetchErr error) (*index, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fetching index and no cache: %w", fetchErr)
	}
	var idx index
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("decoding cached index: %w", err)
	}
	return &idx, nil
}

// getBytes does a GET and returns the body for a 200, or an error otherwise
func getBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// findTemplate returns the named template from the catalog
func (idx *index) findTemplate(name string) (catalogTemplate, bool) {
	for _, t := range idx.Templates {
		if t.Name == name {
			return t, true
		}
	}
	return catalogTemplate{}, false
}
