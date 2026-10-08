package desktop

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// lifecycle adapts a service's plain startup and shutdown functions to the
// hooks Wails calls, so a service never names a Wails type. Either may be nil
type lifecycle struct {
	start func()
	stop  func()
}

// ServiceStartup runs the service's startup function
func (l lifecycle) ServiceStartup(context.Context, application.ServiceOptions) error {
	if l.start != nil {
		l.start()
	}
	return nil
}

// ServiceShutdown runs the service's shutdown function
func (l lifecycle) ServiceShutdown() error {
	if l.stop != nil {
		l.stop()
	}
	return nil
}
