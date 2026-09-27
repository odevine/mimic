// Package catalog reads the template catalog published by the templates repo
// and keeps the downloaded template bundles in a local cache
package catalog

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

// IndexURL is the catalog the app fetches to discover templates and versions.
// It is served from the templates repo's default branch, a stable raw URL that
// can later be swapped for a CDN-backed one without any code change here. It is
// a var so a test can point it at a local server
var IndexURL = "https://raw.githubusercontent.com/odevine/mimic-templates/main/index.json"

// Dir is where downloaded bundles and the last-fetched catalog live. It is a
// var so the app can place the cache and a test can redirect it to a temporary
// directory
var Dir = defaultDir

// indexFetchTimeout bounds the catalog fetch. index.json is tiny, so a short
// timeout is enough and a slow network falls back to the cached copy quickly
const indexFetchTimeout = 15 * time.Second

// Index is the whole catalog: every template and its downloadable versions. It
// mirrors the schema tools/reindex writes in the templates repo
type Index struct {
	Schema    int        `json:"schema"`
	Templates []Template `json:"templates"`
}

// Template groups every published version of one template. Versions are
// sorted newest first, so Versions[0] is Latest
type Template struct {
	Name     string    `json:"name"`
	Latest   string    `json:"latest"`
	Versions []Version `json:"versions"`
}

// Version is one downloadable bundle. SHA256 gates the download's
// integrity; MinEngine lets the app filter incompatible versions before
// fetching anything
type Version struct {
	Version   string `json:"version"`
	MinEngine string `json:"minEngine"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

// defaultDir puts the cache under the per-OS user config directory, so it is
// correct on macOS, Linux, and Windows
func defaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "mimic", "templates"), nil
}

// FetchIndex returns the catalog, preferring a fresh copy from the network and
// falling back to the last one cached so the app works offline against what it
// already has. A successful fetch refreshes the cache
func FetchIndex(ctx context.Context) (*Index, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	cachePath := filepath.Join(dir, "index.json")

	fetchCtx, cancel := context.WithTimeout(ctx, indexFetchTimeout)
	defer cancel()
	body, err := getBytes(fetchCtx, IndexURL)
	if err != nil {
		return cachedIndex(cachePath, err)
	}
	var idx Index
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
func cachedIndex(path string, fetchErr error) (*Index, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fetching index and no cache: %w", fetchErr)
	}
	var idx Index
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

// Find returns the named template from the catalog
func (idx *Index) Find(name string) (Template, bool) {
	for _, t := range idx.Templates {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}
