package main

import (
	"os"
	"path/filepath"

	"github.com/odevine/mimic/engine/template/normal"
)

// assetCandidates and fontCandidates list where the real template assets and
// font overrides live, relative to both a repo-root run and a ui-subdir run.
// The real manifest is developer-local, so a checkout without it falls back to
// generated placeholders
var (
	assetCandidates = []string{"assets/normal", filepath.Join("..", "assets", "normal")}
	fontCandidates  = []string{"local-fonts", filepath.Join("..", "local-fonts")}
)

// resolveAssetDir returns the real asset directory when one is present, else it
// generates placeholder assets into a temp directory and returns a cleanup. The
// cleanup is a no-op for the real directory. This mirrors rendercard so the UI
// renders the same frames the CLI does
func resolveAssetDir() (dir string, cleanup func(), err error) {
	if found := firstExistingDir(assetCandidates); found != "" {
		return found, func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "mimic-placeholder-")
	if err != nil {
		return "", func() {}, err
	}
	if err := normal.WritePlaceholderAssets(tmp); err != nil {
		os.RemoveAll(tmp)
		return "", func() {}, err
	}
	return tmp, func() { os.RemoveAll(tmp) }, nil
}

// resolveFontDir returns the first font-override directory that exists, or ""
// to use the engine's embedded default fonts
func resolveFontDir() string {
	return firstExistingDir(fontCandidates)
}

// firstExistingDir returns the first candidate that is an existing directory,
// or "" when none is
func firstExistingDir(candidates []string) string {
	for _, cand := range candidates {
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			return cand
		}
	}
	return ""
}
