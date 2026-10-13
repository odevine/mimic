package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/ui/internal/workspace"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

func engineFont(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "engine", "fonts", "embedded", "m15", name))
	if err != nil {
		t.Skipf("engine fonts not in this checkout: %v", err)
	}
	return raw
}

func putFont(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A font added when each role had a folder of its own is kept, and the role keeps
// drawing with it
func TestFontsAddedPerRoleBecomeAssignmentsOnStartup(t *testing.T) {
	workspacetest.WithEmptyAssetChain(t)
	cfg, _ := workspace.UserConfigDir()
	fonts := filepath.Join(cfg, "mimic", "fonts")
	putFont(t, filepath.Join(fonts, "title", "Mine.otf"), engineFont(t, "Gotham-Medium.otf"))
	putFont(t, filepath.Join(fonts, "body-italic", "Slanted.ttf"), engineFont(t, "plantin-italic.ttf"))

	ws := workspace.New(workspace.Options{})
	defer ws.Close()

	got := ws.Prefs.Settings().FontOverrides
	if got["title"] != "Gotham Medium" || got["body-italic"] != "Plantin MT Pro Italic" || len(got) != 2 {
		t.Errorf("assignments = %v, want each role on the font it had", got)
	}
	for _, name := range []string{"Mine.otf", "Slanted.ttf"} {
		if _, err := os.Stat(filepath.Join(fonts, name)); err != nil {
			t.Errorf("%s did not come up to the fonts folder: %v", name, err)
		}
	}
	if names := ws.FontPool.Get().Fonts(); len(names) != 2 {
		t.Errorf("pool = %+v, want both fonts", names)
	}
}

// What a person assigned since is not undone by an old folder that turns up again
func TestStartupKeepsAnAssignmentAlreadyMade(t *testing.T) {
	workspacetest.WithEmptyAssetChain(t)
	cfg, _ := workspace.UserConfigDir()
	fonts := filepath.Join(cfg, "mimic", "fonts")

	first := workspace.New(workspace.Options{})
	s := first.Prefs.Settings()
	s.FontOverrides = map[string]string{"info": "Chosen Font Regular"}
	first.Prefs.SetSettings(s)
	first.Close()

	putFont(t, filepath.Join(fonts, "info", "Old.otf"), engineFont(t, "Gotham-Medium.otf"))
	second := workspace.New(workspace.Options{})
	defer second.Close()
	if got := second.Prefs.Settings().FontOverrides["info"]; got != "Chosen Font Regular" {
		t.Errorf("info = %q, want the assignment that was already made", got)
	}
	if _, err := os.Stat(filepath.Join(fonts, "Old.otf")); err != nil {
		t.Errorf("the old font was not kept: %v", err)
	}
}
