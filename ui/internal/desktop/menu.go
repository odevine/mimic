package desktop

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/odevine/mimic/ui/internal/buildinfo"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// menuEventName is the window event a menu choice reaches the page by
const menuEventName = "menu"

// Where the Help menu points
const (
	readmeURL = "https://github.com/odevine/mimic#readme"
	issuesURL = "https://github.com/odevine/mimic/issues"
)

// The modes the View menu switches between, in the order of ⌘1 to ⌘5
var modeItems = []struct{ key, label string }{
	{"single", "Single"},
	{"list", "From a List"},
	{"art", "From Your Art"},
	{"templates", "Templates"},
	{"run", "Run"},
}

// menu builds the application menu. A choice that changes the page is sent to it
// as a menu event naming the action, so the page has one path for a menu choice
// and for the shortcut that used to do the same thing. On macOS the menu owns
// those shortcuts. On Windows and Linux the page keeps handling its own keys, and
// the menu only shows their text
func (s *shell) menu() *application.Menu {
	mac := runtime.GOOS == "darwin"
	send := func(action string) func(*application.Context) {
		return func(*application.Context) { s.app.Event.Emit(menuEventName, action) }
	}
	// item adds a menu item with its shortcut: bound on macOS, shown elsewhere
	item := func(m *application.Menu, label, shortcut, action string) {
		switch {
		case shortcut == "":
			m.Add(label).OnClick(send(action))
		case mac:
			m.Add(label).SetAccelerator(shortcut).OnClick(send(action))
		default:
			m.Add(label + "\t" + displayShortcut(shortcut)).OnClick(send(action))
		}
	}

	menu := s.app.Menu.New()
	if mac {
		menu.AddRole(application.AppMenu)
	}

	file := menu.AddSubmenu("File")
	item(file, "Save Card", "CmdOrCtrl+S", "save")
	item(file, "Open List…", "CmdOrCtrl+O", "openList")
	item(file, "Choose Output Folder…", "CmdOrCtrl+Shift+O", "chooseOutput")
	if !mac {
		file.AddSeparator()
		file.AddRole(application.Quit)
	}

	menu.AddRole(application.EditMenu)

	view := menu.AddSubmenu("View")
	for i, m := range modeItems {
		item(view, m.label, fmt.Sprintf("CmdOrCtrl+%d", i+1), "mode:"+m.key)
	}

	if mac {
		menu.AddRole(application.WindowMenu)
	}

	help := menu.AddSubmenu("Help")
	help.Add("Mimic on GitHub").OnClick(func(*application.Context) { s.sys.OpenURL(readmeURL) })
	help.Add("Report an Issue").OnClick(func(*application.Context) { s.sys.OpenURL(issuesURL) })
	help.AddSeparator()
	help.Add("About Mimic").OnClick(func(*application.Context) { s.about() })
	return menu
}

// displayShortcut is how a shortcut reads in a menu on Windows and Linux
func displayShortcut(s string) string {
	return strings.ReplaceAll(s, "CmdOrCtrl", "Ctrl")
}

// about shows the versions the app is made of, and the template it renders with
func (s *shell) about() {
	name, version := s.svc.Workspace.Active()
	template := "none"
	if name != "" {
		template = name
		if version != "" {
			template += " " + version
		}
	}
	s.app.Dialog.Info().AttachToWindow(s.win).SetTitle("About Mimic").SetMessage(fmt.Sprintf(
		"Mimic %s\nEngine %s\nTemplate %s", buildinfo.Version, buildinfo.Engine(), template)).Show()
}
