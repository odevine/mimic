// Command mimic is the desktop app for the mimic card renderer. It opens a
// window where you search Scryfall with full query syntax, edit a match's
// fields, preview it through the normal template, render whole lists, and save
// the results. The window shows the embedded frontend through the system
// webview, and nothing listens on the network
package main

import (
	"flag"
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

	if *smoke {
		scratch, err := os.MkdirTemp("", "mimic-smoke-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(scratch)
		workspace.UserConfigDir = func() (string, error) { return filepath.Join(scratch, "config"), nil }
	}

	ws := workspace.New(workspace.Options{FontDir: *fontDir})
	defer ws.Close()

	return desktop.Launch(desktop.Options{
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
		Frontend: staticFS(),
		Smoke:    *smoke,
	})
}
