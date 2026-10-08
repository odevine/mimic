// Package desktop is the native shell. It registers the services with Wails,
// opens the window, serves the frontend and the images to it, carries job events
// across to the page, and owns what makes the app an application: the window's
// remembered place, the menus, the single instance, the close prompt, system
// notifications and updates. It is the only package that imports Wails, so the
// rest of the app builds and tests without cgo
package desktop

import (
	_ "embed"
	"errors"
	"image"
	"io/fs"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/odevine/mimic/ui/internal/buildinfo"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/services/cards"
	"github.com/odevine/mimic/ui/internal/services/data"
	"github.com/odevine/mimic/ui/internal/services/list"
	"github.com/odevine/mimic/ui/internal/services/overrides"
	"github.com/odevine/mimic/ui/internal/services/render"
	"github.com/odevine/mimic/ui/internal/services/run"
	"github.com/odevine/mimic/ui/internal/services/settings"
	"github.com/odevine/mimic/ui/internal/services/templates"
	"github.com/odevine/mimic/ui/internal/workspace"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed smoke.html
var smokeHTML []byte

// smokeTimeout is how long the launch check has to report
const smokeTimeout = 90 * time.Second

// appID identifies the app to the operating system. A second launch finds the
// first by it, and it is the bundle identifier of the packaged app
const appID = "io.github.odevine.mimic"

// Services are the services the window's page calls. Run, Data and Templates
// start their background work and stop it through Startup and Shutdown
type Services struct {
	Workspace *workspace.Workspace
	Cards     *cards.Service
	Render    *render.Service
	List      *list.Service
	Run       *run.Service
	Templates *templates.Service
	Settings  *settings.Service
	Data      *data.Service
	Overrides *overrides.Service
}

// Options is what the shell needs from the binary around it
type Options struct {
	Services Services
	// Frontend is the page's files, rooted at the site root
	Frontend fs.FS
	// Smoke runs the launch check instead of the app: the window loads a page
	// that exercises the bridge, the events and the image routes, and the app
	// exits when it reports. It uses no remembered window, takes no single
	// instance lock, and sends no notification or update check
	Smoke bool
}

// OpenPath shows a file or folder in the system file manager. The services that
// open folders take it, and it works once the app exists
func OpenPath(path string) error { return application.Get().Browser.OpenFile(path) }

// shell is the running app: the Wails application, its window and the services
// the native pieces reach into
type shell struct {
	app   *application.App
	win   application.Window
	svc   Services
	sys   *System
	smoke bool
	// quitting is set once the user has agreed to quit, so the close prompt does
	// not ask again on the way out
	quitting atomic.Bool
}

// Launch opens the window and blocks until the app quits. In a launch check it
// returns the check's verdict
func Launch(o Options) error {
	sh := &shell{svc: o.Services, smoke: o.Smoke, sys: &System{}}
	if o.Smoke {
		sh.sys.smoke = &smoke{done: make(chan error, 1)}
	}
	svc := o.Services
	ws := svc.Workspace
	img := images{render: svc.Render, run: svc.Run, data: svc.Data}

	updates := newUpdates(sh)
	notifier := newNotifier(sh)
	svc.Run.OnFinished(notifier.runFinished)

	bound := []application.Service{
		application.NewService(&Cards{svc.Cards}),
		application.NewService(&Render{svc.Render}),
		application.NewService(&List{svc.List}),
		application.NewService(&Run{runAPI: svc.Run, lifecycle: lifecycle{stop: svc.Run.Shutdown}}),
		application.NewService(&Templates{templatesAPI: svc.Templates, lifecycle: startIf(!o.Smoke, svc.Templates.Startup)}),
		application.NewService(&Settings{svc.Settings}),
		application.NewService(&Overrides{svc.Overrides}),
		application.NewService(&Data{dataAPI: svc.Data, lifecycle: startIf(!o.Smoke, svc.Data.Startup)}),
		application.NewService(&Updates{updatesAPI: updates, lifecycle: startIf(!o.Smoke, updates.startup)}),
		application.NewService(jobs.NewService(ws.Jobs)),
		application.NewService(sh.sys),
	}
	if !o.Smoke {
		bound = append(bound, application.NewService(notifier))
	}

	opts := application.Options{
		Name:        "Mimic",
		Description: "Mimic renders Magic: The Gathering cards.",
		Services:    bound,
		Assets: application.AssetOptions{
			Handler:        assetHandler(img, o.Frontend, o.Smoke),
			DisableLogging: true,
		},
		Mac:        application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		ShouldQuit: sh.shouldQuit,
	}
	// A test build runs beside the developer's own copy, so it takes no lock that
	// would hand its launch to that copy
	if !o.Smoke && !buildinfo.TestBuild {
		opts.SingleInstance = &application.SingleInstanceOptions{
			UniqueID:               appID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { sh.raise() },
		}
	}
	sh.app = application.New(opts)
	sh.sys.app = sh.app
	updates.init()
	ws.Jobs.SetEmitter(newEmitter(func(name string, data any) { sh.app.Event.Emit(name, data) }))

	start := "/"
	if o.Smoke {
		start = "/smoke.html"
	}
	sh.win = sh.app.Window.NewWithOptions(sh.windowOptions(start))
	sh.sys.window = sh.win
	if !o.Smoke {
		sh.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			sh.place()
			sh.trackWindow()
		})
		sh.app.Menu.SetApplicationMenu(sh.menu())
	}
	sh.watchDrops()

	var verdict error
	if sh.sys.smoke != nil {
		timer := time.AfterFunc(smokeTimeout, func() { sh.sys.smoke.done <- errors.New("launch check timed out") })
		go func() {
			verdict = <-sh.sys.smoke.done
			timer.Stop()
			if verdict != nil {
				// Quitting can end the process before Run returns, so a failed
				// check exits on its own
				log.Println("mimic:", verdict)
				os.Exit(1)
			}
			sh.app.Quit()
		}()
	}

	if err := sh.app.Run(); err != nil {
		return err
	}
	return verdict
}

func startIf(on bool, f func()) lifecycle {
	if on {
		return lifecycle{start: f}
	}
	return lifecycle{}
}

// images adapts the services to the asset handler's Images
type images struct {
	render *render.Service
	run    *run.Service
	data   *data.Service
}

func (i images) RenderImage(id string) (image.Image, string, error) { return i.render.Image(id) }
func (i images) RunFile(id string, n int) (string, error)           { return i.run.File(id, n) }
func (i images) CardbackFile() (string, error)                      { return i.run.CardbackFile() }
func (i images) Symbol(code string, px int, known string) (image.Image, string, error) {
	return i.data.Symbol(code, px, known)
}
