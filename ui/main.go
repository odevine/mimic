// Command mimic is the desktop app for the mimic card renderer. It opens a
// window where you search Scryfall with full query syntax, edit a match's
// fields, preview it through the normal template, render whole lists, and save
// the results. The window shows the embedded frontend through the system
// webview, and nothing listens on the network
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/odevine/mimic/ui/internal/buildinfo"
	"github.com/odevine/mimic/ui/internal/desktop"
	"github.com/odevine/mimic/ui/internal/services/cards"
	"github.com/odevine/mimic/ui/internal/services/data"
	"github.com/odevine/mimic/ui/internal/services/list"
	"github.com/odevine/mimic/ui/internal/services/overrides"
	"github.com/odevine/mimic/ui/internal/services/render"
	"github.com/odevine/mimic/ui/internal/services/run"
	"github.com/odevine/mimic/ui/internal/services/settings"
	"github.com/odevine/mimic/ui/internal/services/templates"
	"github.com/odevine/mimic/ui/internal/testbuild"
	"github.com/odevine/mimic/ui/internal/workspace"
)

func main() {
	if err := launch(); err != nil {
		log.Fatalf("mimic: %v", err)
	}
}

func launch() error {
	fontDir := flag.String("fonts", "", "folder of font overrides, one subfolder per role, used ahead of the app's own fonts folder")
	smoke := flag.Bool("smoke", false, "run the launch check against a scratch config folder and exit")
	flag.Parse()

	if buildinfo.TestBuild {
		if err := testbuild.Apply(os.Getenv); err != nil {
			return err
		}
	}

	// smokeOut is where the launch check's run writes its cards, in the scratch
	// folder it deletes afterward
	smokeOut := ""
	if *smoke {
		scratch, err := os.MkdirTemp("", "mimic-smoke-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(scratch)
		workspace.UserConfigDir = func() (string, error) { return filepath.Join(scratch, "config"), nil }
		smokeOut = filepath.Join(scratch, "out")
	}

	ws := workspace.New(workspace.Options{FontDir: *fontDir})
	defer ws.Close()
	if smokeOut != "" {
		settings := ws.Prefs.Settings()
		settings.OutputDir = smokeOut
		ws.Prefs.SetSettings(settings)
	}

	scripted := ""
	if buildinfo.TestBuild {
		scripted = filepath.Join(os.Getenv(testbuild.EnvHome), "dialogs")
	}

	err := desktop.Launch(desktop.Options{
		Services: desktop.Services{
			Workspace: ws,
			Cards:     cards.New(ws),
			Render:    render.New(ws),
			List:      list.New(ws),
			Run:       run.New(ws, desktop.OpenPath),
			Templates: templates.New(ws),
			Settings:  settings.New(ws),
			Data:      data.New(ws, desktop.OpenPath),
			Overrides: overrides.New(ws),
		},
		Frontend:        staticFS(),
		Smoke:           *smoke,
		ScriptedDialogs: scripted,
	})
	if err == nil && smokeOut != "" {
		err = checkSmokeOutput(smokeOut)
	}
	return err
}

// checkSmokeOutput confirms the launch check's run left a card on disk, which the
// page cannot see for itself
func checkSmokeOutput(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.[jp][pn]g"))
	if err != nil || len(files) == 0 {
		return fmt.Errorf("the launch check's run wrote no card to %s", dir)
	}
	info, err := os.Stat(files[0])
	if err != nil || info.Size() < 1000 {
		return fmt.Errorf("the launch check's card %s is empty", files[0])
	}
	return nil
}
