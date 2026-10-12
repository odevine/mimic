// Package workspacetest builds workspaces over placeholder template assets, with
// prefs in a temporary directory, for the tests of the services
package workspacetest

import (
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/templatetest"
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

// ActiveID is the id of the template a workspace from New starts on
const ActiveID = "test.active"

// New builds a workspace over a standard template installed as a loose developer
// folder. It is enough for everything the resolution and settings services touch
func New(t *testing.T) *workspace.Workspace {
	t.Helper()
	tmp := WithEmptyAssetChain(t)
	assets := filepath.Join(tmp, "assets")
	templatetest.Spec{ID: ActiveID}.WriteLoose(t, filepath.Join(assets, "active"))
	pipeline.LooseDirBases = []string{assets}

	at, _, err := pipeline.Resolve(ActiveID)
	if err != nil {
		t.Fatalf("pipeline.Resolve: %v", err)
	}
	t.Cleanup(at.Close)

	p := workspace.LoadPrefs()
	pipe := pipeline.New(nil, p.FaceTemplates)
	pipe.SetFeatures(p.Features)
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

// TransformTemplate renders the two faces of a transform card
var TransformTemplate = templatetest.Spec{ID: "test.transform", Shapes: templatetest.Transform}

// NewWithTransform is NewSmall with a transform template installed as a loose
// developer folder beside the active template
func NewWithTransform(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws := NewSmall(t)
	TransformTemplate.WriteLoose(t, filepath.Join(pipeline.LooseDirBases[0], "transform"))
	t.Cleanup(ws.Pipe.Close)
	return ws
}

// NativeDPI is what the active template reports as its authored resolution
func NativeDPI(t *testing.T, ws *workspace.Workspace) int {
	t.Helper()
	return ws.Pipe.Layout().NativeDPI()
}
