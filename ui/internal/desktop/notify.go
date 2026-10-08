package desktop

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// notifier posts a system notification when a run finishes while the window is
// not in front. It registers with Wails for the lifecycle only, and a platform
// that cannot post notifications, such as a session with no notification
// service, leaves it switched off rather than stopping the app
type notifier struct {
	sh *shell

	mu       sync.Mutex
	svc      *notifications.NotificationService
	disabled bool
	asked    bool
}

func newNotifier(sh *shell) *notifier { return &notifier{sh: sh} }

// ServiceStartup starts the platform's notification service
func (n *notifier) ServiceStartup(ctx context.Context, opts application.ServiceOptions) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	svc := notifications.New()
	if err := svc.ServiceStartup(ctx, opts); err != nil {
		log.Printf("mimic: notifications are off: %v", err)
		n.disabled = true
		return nil
	}
	n.svc = svc
	return nil
}

// ServiceShutdown stops the platform's notification service
func (n *notifier) ServiceShutdown() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.svc == nil {
		return nil
	}
	return n.svc.ServiceShutdown()
}

// runFinished tells the user a run ended, when they are not looking at the window
// and have not turned notifications off
func (n *notifier) runFinished(v batch.View) {
	if !prefs.On(n.sh.svc.Workspace.Prefs.Settings().NotifyRunDone) || n.sh.win == nil || n.sh.win.IsFocused() {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.disabled || n.svc == nil {
		return
	}
	if !n.asked {
		n.asked = true
		if ok, err := n.svc.CheckNotificationAuthorization(); err == nil && !ok {
			n.svc.RequestNotificationAuthorization()
		}
	}
	title, body := runSummary(v)
	err := n.svc.SendNotification(notifications.NotificationOptions{ID: "run-" + v.ID, Title: title, Body: body})
	if err != nil {
		log.Printf("mimic: sending the notification: %v", err)
	}
}

// runSummary is the notification's text: how the run ended and how many cards
// rendered and failed, and nothing from the cards themselves
func runSummary(v batch.View) (title, body string) {
	counts := batch.Counts(v.Cards)
	title = "Run finished"
	if v.Stopped {
		title = "Run stopped"
	}
	body = fmt.Sprintf("%d %s rendered", counts[batch.StatusDone], plural(counts[batch.StatusDone], "card", "cards"))
	var extra []string
	if n := counts[batch.StatusFailed]; n > 0 {
		extra = append(extra, fmt.Sprintf("%d failed", n))
	}
	if n := counts[batch.StatusUnsupported]; n > 0 {
		extra = append(extra, fmt.Sprintf("%d unsupported", n))
	}
	if n := counts[batch.StatusSkipped]; n > 0 {
		extra = append(extra, fmt.Sprintf("%d skipped", n))
	}
	if len(extra) > 0 {
		body += ", " + strings.Join(extra, ", ")
	}
	return title, body
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
