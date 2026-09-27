package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/odevine/mimic/ui/internal/carddata"
)

// remoteTTL is how long the bulk index is trusted before the settings panel asks
// Scryfall again. It changes once a day
const remoteTTL = time.Hour

// remoteCache remembers the bulk index for remoteTTL
type remoteCache struct {
	mu   sync.Mutex
	at   time.Time
	info *carddata.Remote
}

// remote returns the bulk index, from the cache when it is fresh
func (s *server) remote(ctx context.Context) (*carddata.Remote, error) {
	s.remoteCards.mu.Lock()
	defer s.remoteCards.mu.Unlock()
	if s.remoteCards.info != nil && time.Since(s.remoteCards.at) < remoteTTL {
		return s.remoteCards.info, nil
	}
	info, err := carddata.FetchRemote(ctx, s.scryfall)
	if err != nil {
		return nil, err
	}
	s.remoteCards.info, s.remoteCards.at = info, time.Now()
	return info, nil
}

// cardDataView is what GET /api/carddata returns: the installed copy, what a
// download would fetch when Scryfall answered, and the chosen source
type cardDataView struct {
	Source string           `json:"source"`
	Local  carddata.Status  `json:"local"`
	Remote *carddata.Remote `json:"remote,omitempty"`
}

func (s *server) handleCardData(w http.ResponseWriter, r *http.Request) {
	v := cardDataView{Source: s.prefs.Settings().CardData, Local: s.cards.Status()}
	if v.Source == "" {
		v.Source = cardDataAPI
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if info, err := s.remote(ctx); err == nil {
		v.Remote = info
	}
	writeJSON(w, v)
}

// handleCardDataDownload starts a download of the local copy, replacing any
// installed one once the new one is complete
func (s *server) handleCardDataDownload(w http.ResponseWriter, r *http.Request) {
	if s.cards.Dir() == "" {
		http.Error(w, "there is no config folder to keep card data in", http.StatusInternalServerError)
		return
	}
	id, j := s.newJob()
	if !s.cards.BeginDownload(id) {
		http.Error(w, "card data is already downloading", http.StatusConflict)
		return
	}
	go s.downloadCards(j)
	writeJSON(w, map[string]string{"jobId": id})
}

func (s *server) handleCardDataDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.cards.Remove(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, s.cards.Status())
}

// downloadCards fetches a fresh copy and installs it, reporting progress on j
func (s *server) downloadCards(j *job) {
	// Each ending clears the download before its last event, so a page that
	// refreshes on that event never sees it still running
	defer s.cards.EndDownload()
	ctx := context.Background()
	fail := func(err error) {
		s.cards.EndDownload()
		j.emit(jobEvent{Done: true, Err: err.Error()})
	}

	s.remoteCards.mu.Lock()
	s.remoteCards.info = nil
	s.remoteCards.mu.Unlock()
	info, err := s.remote(ctx)
	if err != nil {
		fail(err)
		return
	}

	report := throttleBytes(func(step string, frac float64) {
		// The build finishes just after the last byte arrives, so the bar stops
		// short of full until the copy is installed
		j.emit(jobEvent{Step: step, Frac: frac * 0.97})
	})
	meta, err := s.cards.Fetch(ctx, s.scryfall, info, report)
	if err != nil {
		fail(err)
		return
	}
	j.emit(jobEvent{Step: "Installing", Frac: 0.98})
	if err := s.cards.Install(); err != nil {
		fail(err)
		return
	}
	s.cards.EndDownload()
	j.emit(jobEvent{Done: true, Frac: 1, Step: fmt.Sprintf("Installed %d printings", meta.Printings)})
}
