package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/odevine/mimic/engine/version"
)

// bundleDownloadTimeout bounds a bundle download. Bundles are large (the live
// normal is about 199 MB), so this is generous
const bundleDownloadTimeout = 15 * time.Minute

// bundlePath is where a template version's bundle is cached. Versions sit side
// by side, so an update never overwrites a working bundle and a downgrade stays
// possible. The version is in the filename, so the app never opens a bundle to
// learn what it is
func bundlePath(name, ver string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name, ver+".mimic"), nil
}

// cachedVersions lists the versions of a template already in the cache, newest
// first. Names come from the filenames, never from opening a bundle
func cachedVersions(name string) ([]string, error) {
	dir, err := cacheDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mimic") {
			continue
		}
		versions = append(versions, strings.TrimSuffix(e.Name(), ".mimic"))
	}
	sort.Slice(versions, func(i, j int) bool { return lessVersion(versions[j], versions[i]) })
	return versions, nil
}

// newestCachedBundle returns the path to the newest cached bundle of a template,
// or ok=false when none is cached
func newestCachedBundle(name string) (string, bool) {
	versions, err := cachedVersions(name)
	if err != nil || len(versions) == 0 {
		return "", false
	}
	path, err := bundlePath(name, versions[0])
	if err != nil {
		return "", false
	}
	return path, true
}

// pickCompatible returns the newest version of a template the running engine can
// render. Versions are newest first, so the first compatible one wins
func pickCompatible(t catalogTemplate) (catalogVersion, bool) {
	for _, v := range t.Versions {
		if version.Satisfies(v.MinEngine) {
			return v, true
		}
	}
	return catalogVersion{}, false
}

// ensureTemplate makes sure the latest compatible version of a template is in
// the cache and returns its path. It fetches the catalog, picks the newest
// version the engine can render, uses a cached copy when present, and otherwise
// downloads and verifies one. Errors are returned, not fatal: the caller falls
// back to whatever it already has
func ensureTemplate(ctx context.Context, name string) (string, error) {
	idx, err := fetchIndex(ctx)
	if err != nil {
		return "", err
	}
	t, ok := idx.findTemplate(name)
	if !ok {
		return "", fmt.Errorf("template %q not in catalog", name)
	}
	v, ok := pickCompatible(t)
	if !ok {
		return "", fmt.Errorf("no version of %q is compatible with engine %s", name, version.Version)
	}
	path, err := bundlePath(name, v.Version)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return download(ctx, name, v, nil)
}

// ensureVersion makes sure one specific version of a template is in the cache
// and returns its path. A cached copy is used as is; otherwise the catalog entry
// for that exact version is downloaded and verified. progress may be nil
func ensureVersion(ctx context.Context, name, version string, progress func(done, total int64)) (string, error) {
	if isVersionCached(name, version) {
		return bundlePath(name, version)
	}
	idx, err := fetchIndex(ctx)
	if err != nil {
		return "", err
	}
	t, ok := idx.findTemplate(name)
	if !ok {
		return "", fmt.Errorf("template %q not in catalog", name)
	}
	v, ok := findVersion(t, version)
	if !ok {
		return "", fmt.Errorf("version %s of %q not in catalog", version, name)
	}
	return download(ctx, name, v, progress)
}

// findVersion returns the catalog entry for one exact version of a template
func findVersion(t catalogTemplate, version string) (catalogVersion, bool) {
	for _, v := range t.Versions {
		if v.Version == version {
			return v, true
		}
	}
	return catalogVersion{}, false
}

// isVersionCached reports whether a specific version is already in the cache
func isVersionCached(name, version string) bool {
	path, err := bundlePath(name, version)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// newestCachedVersion returns the newest cached version of a template, or
// ok=false when none is cached
func newestCachedVersion(name string) (string, bool) {
	versions, err := cachedVersions(name)
	if err != nil || len(versions) == 0 {
		return "", false
	}
	return versions[0], true
}

// progressWriter counts bytes written and reports the running total, so a
// download can drive a progress bar without buffering
type progressWriter struct {
	done   int64
	total  int64
	report func(done, total int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.done += int64(len(p))
	w.report(w.done, w.total)
	return len(p), nil
}

// download fetches a bundle to a temp file in the cache, verifies its SHA-256
// against the catalog entry, and only then renames it into place. A mismatch
// discards the file and reports, so a corrupt or truncated download never
// reaches the cache. The download streams through the hash, so a large bundle
// never sits fully in memory. progress, when non-nil, is called with the bytes
// received so far and the total, for a progress bar
func download(ctx context.Context, name string, v catalogVersion, progress func(done, total int64)) (string, error) {
	dst, err := bundlePath(name, v.Version)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "download-*.mimic.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	// Remove the temp file unless it is renamed into place below
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	dlCtx, cancel := context.WithTimeout(ctx, bundleDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, v.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: %s", v.URL, resp.Status)
	}

	// Prefer the catalog size for the total, falling back to the response's
	// Content-Length, so a progress bar has a denominator even when one is absent
	total := v.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	h := sha256.New()
	dst2 := io.MultiWriter(tmp, h)
	if progress != nil {
		dst2 = io.MultiWriter(tmp, h, &progressWriter{total: total, report: progress})
	}
	if _, err := io.Copy(dst2, resp.Body); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(sum, v.SHA256) {
		return "", fmt.Errorf("checksum mismatch for %s %s: got %s, want %s", name, v.Version, sum, v.SHA256)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// lessVersion reports whether semver a precedes b, comparing major, minor, and
// patch numerically. A field that does not parse sorts as zero, which is enough
// for the plain X.Y.Z versions the templates repo emits
func lessVersion(a, b string) bool {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return a < b
}

func parseVersion(s string) [3]int {
	var out [3]int
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	for i, part := range strings.SplitN(strings.TrimPrefix(s, "v"), ".", 3) {
		if i > 2 {
			break
		}
		out[i], _ = strconv.Atoi(part)
	}
	return out
}
