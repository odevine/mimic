package desktop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/buildinfo"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/releases"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// updateRepo is where releases are published
const updateRepo = "odevine/mimic"

// updateKey is the Ed25519 public key releases are signed with, base64 encoded.
// A release build stamps it with -ldflags "-X
// github.com/odevine/mimic/ui/internal/desktop.updateKey=<key>". A build without
// one installs only releases that carry no signature, checked by their digest
var updateKey string

// updateEventName is the window event the page hears the update status by
const updateEventName = "update"

// checkEvery is the shortest gap between two checks at launch
const checkEvery = 24 * time.Hour

// startupCheckDelay holds the check at launch back until the window is up, so it
// never competes with the page loading
const startupCheckDelay = 10 * time.Second

// provider finds ui releases for the Wails updater. The updater's own GitHub
// provider reads only the repository's latest release and strips a leading v from
// its tag, and this repository also publishes engine releases under tags such as
// engine/v0.17.0 and ui releases under ui/v1.0.0, so the lookup lives here and
// the updater keeps the download, verification and swap
type provider struct{ client *releases.Client }

func (p *provider) Name() string { return "github" }

// Check returns the newest ui release when it is newer than the running app and
// has a checked file for this platform, and nothing otherwise. A release published
// before its files and checksums are uploaded reads as no update yet
func (p *provider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	rel, err := p.client.Latest(ctx)
	if errors.Is(err, releases.ErrNone) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !releases.Newer(rel.Version, req.CurrentVersion) {
		return nil, nil
	}
	asset := releases.Pick(rel.Assets, req.Platform, req.Arch)
	if asset == nil {
		return nil, nil
	}
	v := &updater.Verification{}
	if v.Digest, err = p.client.Digest(ctx, rel, asset.Name); err != nil {
		return nil, err
	}
	if v.Digest != nil {
		v.DigestAlgo = "sha256"
	}
	if v.Signature, err = p.client.Signature(ctx, rel, asset.Name); err != nil {
		return nil, err
	}
	if v.Signature != nil {
		v.SignatureAlgo = "ed25519"
	}
	// A file with no digest and no signature has nothing to be checked against.
	// That is a release whose checksums are still being written, so it is not
	// offered yet rather than installed unchecked
	if v.Digest == nil && v.Signature == nil {
		return nil, nil
	}
	out := &updater.Release{
		Version:     rel.Version,
		Name:        rel.Name,
		Notes:       rel.Notes,
		PublishedAt: rel.Published,
		Artifact: updater.Artifact{
			Filename: asset.Name,
			Filetype: strings.TrimPrefix(strings.ToLower(filepath.Ext(asset.Name)), "."),
			Size:     asset.Size,
			Platform: req.Platform,
			Arch:     req.Arch,
		},
		Metadata: map[string]any{"assetURL": asset.URL, "pageURL": rel.URL},
	}
	out.Verification = v
	return out, nil
}

// Download streams the release's file
func (p *provider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	url, _ := rel.Metadata["assetURL"].(string)
	if url == "" {
		return errors.New("the release has no download address")
	}
	return p.client.Download(ctx, &releases.Asset{Name: rel.Artifact.Filename, Size: rel.Artifact.Size, URL: url}, dst, onProgress)
}

// UpdateRelease is the release on offer
type UpdateRelease struct {
	Version string `json:"version"`
	Notes   string `json:"notes,omitempty"`
	// PageURL is the release's page, where the installers are
	PageURL string `json:"pageUrl,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// UpdateStatus is everything the page shows about updates: whether the app can
// check at all, where a check or an install has got to, and the release on offer.
// State is one of unavailable, idle, checking, up-to-date, available,
// downloading, ready or error
type UpdateStatus struct {
	State   string `json:"state"`
	Current string `json:"current"`
	// Reason says why State is unavailable
	Reason  string         `json:"reason,omitempty"`
	Release *UpdateRelease `json:"release,omitempty"`
	// CanInstall is false for an install the app cannot replace itself in, such
	// as a system package, where the page offers the release page instead
	CanInstall bool   `json:"canInstall"`
	Written    int64  `json:"written,omitempty"`
	Total      int64  `json:"total,omitempty"`
	Error      string `json:"error,omitempty"`
	// CheckedAt is when the last check finished, in Unix seconds
	CheckedAt int64 `json:"checkedAt,omitempty"`
}

type updatesAPI interface {
	Status() UpdateStatus
	Check() (UpdateStatus, error)
	Install() error
	Restart() error
}

// Updates is the bound update service
type Updates struct {
	updatesAPI
	lifecycle
}

// updatesService drives the Wails updater headless and keeps the status the page
// reads. It sends the whole status to the page as an update event whenever it
// changes
type updatesService struct {
	sh     *shell
	client *releases.Client
	up     *updater.Updater

	mu     sync.Mutex
	status UpdateStatus
	// swappable is whether this install can replace itself
	swappable bool
}

func newUpdates(sh *shell) *updatesService {
	return &updatesService{sh: sh, client: releases.NewClient(updateRepo)}
}

// init configures the updater, once the app exists
func (u *updatesService) init() {
	u.up = u.sh.app.Updater
	u.status = UpdateStatus{State: "idle", Current: buildinfo.Version}
	if reason := u.unavailable(); reason != "" {
		u.status.State, u.status.Reason = "unavailable", reason
		return
	}
	exe, _ := os.Executable()
	u.swappable = releases.CanSwap(runtime.GOOS, exe, os.Getenv)

	cfg := updater.Config{
		CurrentVersion: buildinfo.Version,
		Providers:      []updater.Provider{&provider{client: u.client}},
		Window:         updater.WindowNone,
	}
	if updateKey != "" {
		if key, err := base64.StdEncoding.DecodeString(updateKey); err == nil {
			cfg.PublicKey = key
		}
	}
	if err := u.up.Init(cfg); err != nil {
		log.Printf("mimic: updates are off: %v", err)
		u.status.State, u.status.Reason = "unavailable", err.Error()
		return
	}
	u.sh.app.Event.On(updater.EventDownloadProgress, func(e *application.CustomEvent) {
		if p, ok := progressOf(e.Data); ok {
			u.update(func(s *UpdateStatus) { s.Written, s.Total = p.Written, p.Total })
		}
	})
}

// progressOf reads a download progress event's payload, which arrives as the
// value itself, a pointer to it, or a one-element list of either
func progressOf(data any) (updater.Progress, bool) {
	switch d := data.(type) {
	case updater.Progress:
		return d, true
	case *updater.Progress:
		if d != nil {
			return *d, true
		}
	case []any:
		if len(d) == 1 {
			return progressOf(d[0])
		}
	case map[string]any:
		raw, err := json.Marshal(d)
		var p updater.Progress
		if err == nil && json.Unmarshal(raw, &p) == nil {
			return p, true
		}
	}
	return updater.Progress{}, false
}

// unavailable says why this build never checks, or is empty when it can. A build
// that was not stamped with a version has nothing to compare a release against
func (u *updatesService) unavailable() string {
	switch {
	case u.sh.smoke:
		return "The launch check does not look for updates"
	// A test build that was given a version checks for updates like a release
	case buildinfo.Version == "" || buildinfo.Version == "dev" || (devBuild && !buildinfo.TestBuild):
		return "A development build does not check for updates"
	}
	return ""
}

// startup checks for an update once the window has had time to appear, when the
// setting is on and the last check was not within a day
func (u *updatesService) startup() {
	if u.Status().State == "unavailable" {
		return
	}
	go func() {
		time.Sleep(startupCheckDelay)
		store := u.sh.svc.Workspace.Prefs
		if !checkDue(time.Now(), store.LastUpdateCheck(), prefs.On(store.Settings().CheckUpdates)) {
			return
		}
		if _, err := u.Check(); err != nil {
			log.Printf("mimic: checking for updates: %v", err)
		}
	}()
}

// checkDue reports whether a check at launch should run
func checkDue(now, last time.Time, enabled bool) bool {
	return enabled && now.Sub(last) >= checkEvery
}

// update changes the status under the lock and sends the result to the page
func (u *updatesService) update(change func(*UpdateStatus)) UpdateStatus {
	u.mu.Lock()
	change(&u.status)
	s := u.status
	u.mu.Unlock()
	u.sh.app.Event.Emit(updateEventName, s)
	return s
}

// Status returns where updates stand
func (u *updatesService) Status() UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

// Check asks GitHub for a newer release and returns the status it leaves
func (u *updatesService) Check() (UpdateStatus, error) {
	if s := u.Status(); s.State == "unavailable" {
		return s, apierr.New(apierr.BadRequest, s.Reason)
	} else if s.State == "checking" || s.State == "downloading" {
		return s, nil
	}
	u.update(func(s *UpdateStatus) { s.State, s.Error = "checking", "" })
	u.sh.svc.Workspace.Prefs.SetLastUpdateCheck(time.Now())

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	rel, err := u.up.Check(ctx)
	now := time.Now().Unix()
	if err != nil {
		u.update(func(s *UpdateStatus) { s.State, s.Error, s.CheckedAt = "error", err.Error(), now })
		return u.Status(), apierr.Wrap(apierr.BadGateway, "checking for updates", err)
	}
	if rel == nil {
		return u.update(func(s *UpdateStatus) { s.State, s.Release, s.CheckedAt = "up-to-date", nil, now }), nil
	}
	page, _ := rel.Metadata["pageURL"].(string)
	return u.update(func(s *UpdateStatus) {
		s.State, s.CheckedAt, s.CanInstall = "available", now, u.swappable
		s.Release = &UpdateRelease{Version: rel.Version, Notes: rel.Notes, PageURL: page, Size: rel.Artifact.Size}
	}), nil
}

// Install downloads the release found by the last check, checks it and stages
// the swap. It returns at once, and the status carries the progress and the end.
// The new version starts when Restart is called
func (u *updatesService) Install() error {
	s := u.Status()
	if s.State != "available" || s.Release == nil {
		return apierr.New(apierr.BadRequest, "there is no update to install")
	}
	if !s.CanInstall {
		return apierr.New(apierr.BadRequest, "this install cannot update itself, so download the new version from the release page")
	}
	u.update(func(s *UpdateStatus) { s.State, s.Written, s.Total, s.Error = "downloading", 0, 0, "" })
	go func() {
		if err := u.up.DownloadAndInstall(context.Background()); err != nil {
			log.Printf("mimic: installing the update: %v", err)
			u.update(func(s *UpdateStatus) { s.State, s.Error = "error", err.Error() })
			return
		}
		u.update(func(s *UpdateStatus) { s.State = "ready" })
	}()
	return nil
}

// Restart starts the staged version. With a run going it asks first, because
// quitting stops the run
func (u *updatesService) Restart() error {
	if u.Status().State != "ready" {
		return apierr.New(apierr.BadRequest, "no update is ready to start")
	}
	if u.sh.svc.Workspace.Run.Active() {
		if !u.sh.sys.Confirm(ConfirmRequest{
			Title:   "Restart Mimic",
			Message: "A run is still going. Restarting stops it, and the cards not yet rendered are skipped.",
			Accept:  "Stop and Restart",
		}) {
			return nil
		}
		u.sh.svc.Run.Shutdown()
	}
	u.sh.quitting.Store(true)
	if err := u.up.Restart(context.Background()); err != nil {
		u.sh.quitting.Store(false)
		return apierr.Wrap(apierr.Internal, "restarting", err)
	}
	return nil
}
