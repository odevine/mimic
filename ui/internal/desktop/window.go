package desktop

import (
	"sync"
	"time"

	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/winstate"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The window opens at this size when there is nothing to restore, and never
// shrinks below the minimum, which is the narrowest layout the CSS lays out
const (
	defaultWidth  = 1280
	defaultHeight = 820
	minWidth      = 720
	minHeight     = 520
)

// saveDelay is how long the window must stay put before its place is written, so
// dragging an edge does not write the file on every step
const saveDelay = 500 * time.Millisecond

// The window's background before the page paints, matching --bg of each theme so
// it does not flash the wrong color
var (
	darkBackground  = application.NewRGB(0x22, 0x1f, 0x22)
	lightBackground = application.NewRGB(0xed, 0xec, 0xec)
)

// windowOptions are the main window's options. Outside a launch check it starts
// hidden, because where it belongs depends on the displays, and the system does
// not report those until the app is running. place puts it right and shows it
func (s *shell) windowOptions(url string) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             "main",
		Title:            "Mimic",
		Width:            defaultWidth,
		Height:           defaultHeight,
		MinWidth:         minWidth,
		MinHeight:        minHeight,
		URL:              url,
		BackgroundColour: s.background(),
		EnableFileDrop:   true,
		DevToolsEnabled:  devBuild,
		Hidden:           !s.smoke,
	}
}

// place moves the window to where the user left it, when that is still on a
// connected display, and otherwise centers it at the default size, then shows it.
// It runs once the app has started and the displays are known
func (s *shell) place() {
	defer s.win.Show()
	saved := s.svc.Workspace.Prefs.WindowState()
	if saved == nil {
		s.win.Center()
		return
	}
	r, ok := winstate.Fit(&winstate.Rect{X: saved.X, Y: saved.Y, Width: saved.Width, Height: saved.Height}, s.waitForDisplays(), winstate.Size{Width: minWidth, Height: minHeight})
	if !ok {
		s.win.Center()
		return
	}
	s.win.SetSize(r.Width, r.Height)
	s.win.SetPosition(r.X, r.Y)
	if saved.Maximised {
		s.win.Maximise()
	}
}

// displays are the bounds of every connected display
func (s *shell) displays() []winstate.Rect {
	var out []winstate.Rect
	for _, d := range s.app.Screen.GetAll() {
		out = append(out, winstate.Rect{X: d.Bounds.X, Y: d.Bounds.Y, Width: d.Bounds.Width, Height: d.Bounds.Height})
	}
	return out
}

// waitForDisplays returns the connected displays. The system fills them in a
// moment after the app starts, so an empty answer is tried again for a short
// while before it is believed
func (s *shell) waitForDisplays() []winstate.Rect {
	for range 60 {
		if d := s.displays(); len(d) > 0 {
			return d
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// background is the window's color before the page paints, by the saved theme.
// The system theme follows the operating system
func (s *shell) background() application.RGBA {
	switch s.svc.Workspace.Prefs.Settings().Theme {
	case "light":
		return lightBackground
	case "system":
		if !s.app.Env.IsDarkMode() {
			return lightBackground
		}
	}
	return darkBackground
}

// trackWindow writes the window's place to prefs shortly after it stops moving
// or changing size. A maximised window keeps the place it had before, so
// un-maximising it later has somewhere to go
func (s *shell) trackWindow() {
	var (
		mu    sync.Mutex
		timer *time.Timer
		// normal is the place of the window when not maximised, which a window
		// restored maximised has from the last session
		normal = s.svc.Workspace.Prefs.WindowState()
	)
	capture := func() {
		mu.Lock()
		defer mu.Unlock()
		x, y := s.win.Position()
		w, h := s.win.Size()
		maximised := s.win.IsMaximised()
		if !maximised || normal == nil {
			normal = &prefs.Window{X: x, Y: y, Width: w, Height: h}
		}
		st := *normal
		st.Maximised = maximised
		s.svc.Workspace.Prefs.SetWindowState(st)
	}
	later := func(*application.WindowEvent) {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(saveDelay, capture)
	}
	for _, e := range []events.WindowEventType{events.Common.WindowDidMove, events.Common.WindowDidResize, events.Common.WindowMaximise, events.Common.WindowUnMaximise, events.Common.WindowRestore} {
		s.win.OnWindowEvent(e, later)
	}
	// Record where the window actually landed, which differs from what was saved
	// when that place was off every display
	later(nil)
	// The pending write is made at once when the window closes
	s.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !s.mayClose() {
			e.Cancel()
			return
		}
		capture()
	})
}

// raise brings the window to the front, as a second launch asks
func (s *shell) raise() {
	s.win.UnMinimise()
	s.win.Show()
	s.win.Focus()
}

// mayClose reports whether the app may quit now. With a run going it asks first
// and, if the user agrees, stops the run and quits itself, so the answer here is
// no. The prompt runs off the caller, which is in the middle of a window event
func (s *shell) mayClose() bool {
	if s.quitting.Load() || s.smoke || !s.svc.Workspace.Run.Active() {
		return true
	}
	go func() {
		if !s.sys.Confirm(ConfirmRequest{
			Title:   "Quit Mimic",
			Message: "A run is still going. Quitting stops it, and the cards not yet rendered are skipped.",
			Accept:  "Stop and Quit",
		}) {
			return
		}
		s.quitting.Store(true)
		s.svc.Run.Shutdown()
		s.app.Quit()
	}()
	return false
}

// shouldQuit is the application's own quit, such as ⌘Q, which does not pass
// through the window's closing
func (s *shell) shouldQuit() bool { return s.mayClose() }

// watchDrops tells the page the paths of files dropped on the window
func (s *shell) watchDrops() {
	s.win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		s.app.Event.Emit("files-dropped", e.Context().DroppedFiles())
	})
}
