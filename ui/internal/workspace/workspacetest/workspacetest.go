// Package workspacetest builds workspaces over placeholder template assets, with
// prefs in a temporary directory, for the tests of the services
package workspacetest

import (
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// WithEmptyAssetChain points the loose-dir roots at nothing and the config
// directory at a fresh temp dir, so a test drives the fallback tiers
// deterministically and never touches the real prefs or cache. It returns the
// temp dir
func WithEmptyAssetChain(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	oldBases, oldCfg := pipeline.LooseDirBases, workspace.UserConfigDir
	pipeline.LooseDirBases = []string{filepath.Join(tmp, "no-such-assets")}
	workspace.UserConfigDir = func() (string, error) { return filepath.Join(tmp, "config"), nil }
	t.Cleanup(func() { pipeline.LooseDirBases, workspace.UserConfigDir = oldBases, oldCfg })
	return tmp
}

// New builds a workspace over the placeholder normal template. It is enough for
// everything the resolution and settings services touch
func New(t *testing.T) *workspace.Workspace {
	t.Helper()
	WithEmptyAssetChain(t)

	at, _, err := pipeline.Resolve("normal")
	if err != nil {
		t.Fatalf("pipeline.Resolve: %v", err)
	}
	t.Cleanup(at.Close)

	p := workspace.LoadPrefs()
	pipe := pipeline.New(nil, p.FaceTemplates)
	pipe.Install(at)
	return &workspace.Workspace{Pipe: pipe, Prefs: p, Jobs: jobs.NewRegistry()}
}

// NewSmall is New with the output resolution set low, so a batch test finishes
// quickly with no network
func NewSmall(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws := New(t)
	ws.Prefs.SetResolution(workspace.DefaultPreviewDPI, 30)
	return ws
}

// NewWithTransform is NewSmall with transform installed as a loose developer
// folder of placeholder assets beside the active normal template
func NewWithTransform(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws := NewSmall(t)
	assets := filepath.Join(t.TempDir(), "assets")
	if err := pipeline.WritePlaceholders(filepath.Join(assets, "transform"), "transform"); err != nil {
		t.Fatal(err)
	}
	pipeline.LooseDirBases = []string{assets}
	t.Cleanup(ws.Pipe.Close)
	return ws
}

// NativeDPI is what the active template reports as its authored resolution
func NativeDPI(t *testing.T, ws *workspace.Workspace) int {
	t.Helper()
	m, err := ws.Pipe.Manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	return m.NativeDPI()
}
