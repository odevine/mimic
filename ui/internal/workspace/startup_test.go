package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/templatetest"
	"github.com/odevine/mimic/ui/internal/workspace"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

// A settings file written by the release before templates were loaded from what
// is installed starts the app on the template that replaced the ones it names
func TestAWorkspaceStartsFromAnEarlierReleasesSettings(t *testing.T) {
	tmp := workspacetest.WithEmptyAssetChain(t)
	assets := filepath.Join(tmp, "assets")
	templatetest.Spec{ID: prefs.DefaultTemplate}.WriteLoose(t, filepath.Join(assets, "m15"))
	pipeline.LooseDirBases = []string{assets}

	raw, err := os.ReadFile(filepath.Join("..", "prefs", "testdata", "prefs-0.9.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := workspace.UserConfigDir()
	if err := os.MkdirAll(filepath.Join(cfg, "mimic"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "mimic", "prefs.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(workspace.Options{})
	defer ws.Close()

	name, version := ws.Active()
	if name != prefs.DefaultTemplate || version != pipeline.LocalVersion {
		t.Errorf("active = %s %s, want %s local", name, version, prefs.DefaultTemplate)
	}
	if ws.Startup.Source != pipeline.SourceExplicit {
		t.Errorf("source = %d, want the restored choice", ws.Startup.Source)
	}
	// Every face the old settings chose a template for still has one
	for _, sh := range []string{"single/standard"} {
		if got := ws.Pipe.FaceTemplates()[sh]; got != prefs.DefaultTemplate {
			t.Errorf("%s renders with %q", sh, got)
		}
	}
	if got, _ := ws.Prefs.Template(); got != prefs.DefaultTemplate {
		t.Errorf("prefs still name %q", got)
	}
}
