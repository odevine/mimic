// Package data is the service behind the Settings panel's local card data and
// fonts rows, and the mana symbol pips
package data

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/carddata"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/progress"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// remoteTTL is how long the bulk index is trusted before the settings panel asks
// Scryfall again. It changes once a day
const remoteTTL = time.Hour

// Service manages the local copy of Scryfall data, the fonts folder and the
// symbol renderer
type Service struct {
	ws   *workspace.Workspace
	open func(path string) error

	// remote caches Scryfall's bulk data index for the settings panel
	remoteMu sync.Mutex
	remoteAt time.Time
	remote   *carddata.Remote

	// pips is the mana symbol renderer the editor's pips draw with, rebuilt
	// when the mana font changes
	pips pipCache
}

// New returns the service over a workspace. open shows a folder in the system
// file browser
func New(ws *workspace.Workspace, open func(path string) error) *Service {
	return &Service{ws: ws, open: open}
}

// Startup loads the installed copy of card data in the background
func (s *Service) Startup() { go s.ws.Cards.Load() }

// remoteIndex returns the bulk index, from the cache when it is fresh
func (s *Service) remoteIndex(ctx context.Context) (*carddata.Remote, error) {
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	if s.remote != nil && time.Since(s.remoteAt) < remoteTTL {
		return s.remote, nil
	}
	info, err := carddata.FetchRemote(ctx, s.ws.HTTP)
	if err != nil {
		return nil, err
	}
	s.remote, s.remoteAt = info, time.Now()
	return info, nil
}

// CardDataView is the installed copy, what a download would fetch when Scryfall
// answered, and the chosen source
type CardDataView struct {
	Source string           `json:"source"`
	Local  carddata.Status  `json:"local"`
	Remote *carddata.Remote `json:"remote,omitempty"`
}

// CardData reports the local copy of card data
func (s *Service) CardData(ctx context.Context) CardDataView {
	v := CardDataView{Source: s.ws.Prefs.Settings().CardData, Local: s.ws.Cards.Status()}
	if v.Source == "" {
		v.Source = workspace.CardDataAPI
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if info, err := s.remoteIndex(ctx); err == nil {
		v.Remote = info
	}
	return v
}

// DownloadCardData starts a download of the local copy and returns its job. The
// new copy replaces any installed one once it is complete
func (s *Service) DownloadCardData() (string, error) {
	if s.ws.Cards.Dir() == "" {
		return "", apierr.New(apierr.Internal, "there is no config folder to keep card data in")
	}
	j := s.ws.Jobs.New()
	if !s.ws.Cards.BeginDownload(j.ID()) {
		return "", apierr.New(apierr.Conflict, "card data is already downloading")
	}
	go s.download(j)
	return j.ID(), nil
}

// RemoveCardData deletes the installed copy
func (s *Service) RemoveCardData() (carddata.Status, error) {
	if err := s.ws.Cards.Remove(); err != nil {
		return carddata.Status{}, apierr.Wrap(apierr.Conflict, "", err)
	}
	return s.ws.Cards.Status(), nil
}

// download fetches a fresh copy and installs it, reporting progress on j
func (s *Service) download(j *jobs.Job) {
	// Each ending clears the download before its last event, so a page that
	// refreshes on that event never sees it still running
	defer s.ws.Cards.EndDownload()
	ctx := context.Background()
	fail := func(err error) {
		s.ws.Cards.EndDownload()
		j.Emit(jobs.Event{Done: true, Err: err.Error()})
	}
	s.remoteMu.Lock()
	s.remote = nil
	s.remoteMu.Unlock()
	info, err := s.remoteIndex(ctx)
	if err != nil {
		fail(err)
		return
	}
	report := progress.Bytes(func(step string, frac float64) {
		// The build finishes just after the last byte arrives, so the bar stops
		// short of full until the copy is installed
		j.Emit(jobs.Event{Step: step, Frac: frac * 0.97})
	})
	meta, err := s.ws.Cards.Fetch(ctx, s.ws.HTTP, info, report)
	if err != nil {
		fail(err)
		return
	}
	j.Emit(jobs.Event{Step: "Installing", Frac: 0.98})
	if err := s.ws.Cards.Install(); err != nil {
		fail(err)
		return
	}
	s.ws.Cards.EndDownload()
	j.Emit(jobs.Event{Done: true, Frac: 1, Step: fmt.Sprintf("Installed %d printings", meta.Printings)})
}
