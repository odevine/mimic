package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// bulkListURL is Scryfall's index of bulk data files. A test points it at a
// local server
var bulkListURL = "https://api.scryfall.com/bulk-data"

// remoteTTL is how long the bulk index is trusted before the settings panel asks
// Scryfall again. It changes once a day
const remoteTTL = time.Hour

// bulkFile is one entry of Scryfall's bulk data index
type bulkFile struct {
	Type      string    `json:"type"`
	UpdatedAt time.Time `json:"updated_at"`
	URI       string    `json:"jsonl_download_uri"`
	Size      int64     `json:"compressed_size"`
}

// remoteCards is what a download would fetch, as the settings panel shows it
type remoteCards struct {
	UpdatedAt time.Time `json:"updatedAt"`
	Size      int64     `json:"size"`

	oracle, cards bulkFile
}

// remoteCache remembers the bulk index for remoteTTL
type remoteCache struct {
	mu   sync.Mutex
	at   time.Time
	info *remoteCards
}

// fetchRemoteCards reads Scryfall's bulk index for the two files a local copy
// is built from
func fetchRemoteCards(ctx context.Context, client *http.Client) (*remoteCards, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bulkListURL, nil)
	if err != nil {
		return nil, err
	}
	setScryfallHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading Scryfall's bulk data list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reading Scryfall's bulk data list: %s", resp.Status)
	}
	var list struct {
		Data []bulkFile `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&list); err != nil {
		return nil, fmt.Errorf("reading Scryfall's bulk data list: %w", err)
	}
	out := &remoteCards{}
	for _, f := range list.Data {
		switch f.Type {
		case "oracle_cards":
			out.oracle = f
		case "default_cards":
			out.cards = f
		}
	}
	if out.oracle.URI == "" || out.cards.URI == "" {
		return nil, errors.New("Scryfall's bulk data list has no oracle_cards or default_cards file")
	}
	out.UpdatedAt = out.cards.UpdatedAt
	out.Size = out.oracle.Size + out.cards.Size
	return out, nil
}

// setScryfallHeaders sets the headers Scryfall asks every client to send
func setScryfallHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "mimic (+https://github.com/odevine/mimic)")
	req.Header.Set("Accept", "application/json")
}

// remote returns the bulk index, from the cache when it is fresh
func (s *server) remote(ctx context.Context) (*remoteCards, error) {
	s.remoteCards.mu.Lock()
	defer s.remoteCards.mu.Unlock()
	if s.remoteCards.info != nil && time.Since(s.remoteCards.at) < remoteTTL {
		return s.remoteCards.info, nil
	}
	info, err := fetchRemoteCards(ctx, s.scryfall)
	if err != nil {
		return nil, err
	}
	s.remoteCards.info, s.remoteCards.at = info, time.Now()
	return info, nil
}

// cardDataView is what GET /api/carddata returns: the installed copy, what a
// download would fetch when Scryfall answered, and the chosen source
type cardDataView struct {
	Source string       `json:"source"`
	Local  localStatus  `json:"local"`
	Remote *remoteCards `json:"remote,omitempty"`
}

func (s *server) handleCardData(w http.ResponseWriter, r *http.Request) {
	v := cardDataView{Source: s.prefs.settings().CardData, Local: s.cards.status()}
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
	if s.cards.dir == "" {
		http.Error(w, "there is no config folder to keep card data in", http.StatusInternalServerError)
		return
	}
	id, j := s.newJob()
	if !s.cards.beginDownload(id) {
		http.Error(w, "card data is already downloading", http.StatusConflict)
		return
	}
	go s.downloadCards(j)
	writeJSON(w, map[string]string{"jobId": id})
}

func (s *server) handleCardDataDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.cards.remove(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, s.cards.status())
}

// downloadCards fetches both bulk files and builds the store from them as they
// stream in, so neither download is ever held whole in memory or on disk. The
// new copy is built beside the installed one and swapped in only when complete
func (s *server) downloadCards(j *job) {
	// Each ending clears the download before its last event, so a page that
	// refreshes on that event never sees it still running
	defer s.cards.endDownload()
	ctx := context.Background()
	fail := func(err error) {
		s.cards.endDownload()
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

	var read atomic.Int64
	report := throttleBytes(func(step string, frac float64) {
		// The build finishes just after the last byte arrives, so the bar stops
		// short of full until the copy is installed
		j.emit(jobEvent{Step: step, Frac: frac * 0.97})
	})
	counted := func(file bulkFile) io.Reader {
		return &lazyReader{open: func() (io.ReadCloser, error) {
			body, err := s.openBulk(ctx, file.URI)
			if err != nil {
				return nil, err
			}
			z, err := gzip.NewReader(&countingReader{r: body, n: &read, report: func(n int64) { report(n, info.Size) }})
			if err != nil {
				body.Close()
				return nil, fmt.Errorf("reading %s: %w", file.Type, err)
			}
			return readCloser{z, body}, nil
		}}
	}
	oracle, cards := counted(info.oracle), counted(info.cards)
	defer oracle.(*lazyReader).Close()
	defer cards.(*lazyReader).Close()

	next := filepath.Join(s.cards.dir, "next")
	os.RemoveAll(next)
	meta, err := buildLocalCards(ctx, next, oracle, cards, info.UpdatedAt, nil)
	if err != nil {
		os.RemoveAll(next)
		fail(err)
		return
	}
	j.emit(jobEvent{Step: "Installing", Frac: 0.98})
	if err := s.cards.install(next); err != nil {
		fail(err)
		return
	}
	s.cards.endDownload()
	j.emit(jobEvent{Done: true, Frac: 1, Step: fmt.Sprintf("Installed %d printings", meta.Printings)})
}

// openBulk starts one bulk file download
func (s *server) openBulk(ctx context.Context, uri string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	setScryfallHeaders(req)
	resp, err := s.scryfall.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading card data: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("downloading card data: %s", resp.Status)
	}
	return resp.Body, nil
}

// beginDownload marks a download as running, refusing a second one
func (s *localStore) beginDownload(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobID != "" {
		return false
	}
	s.jobID = id
	return true
}

func (s *localStore) endDownload() {
	s.mu.Lock()
	s.jobID = ""
	s.mu.Unlock()
}

// install swaps a freshly built copy in for the installed one and loads it.
// Readers hold the read lock through a record read, so the old file is never
// closed under one
func (s *localStore) install(next string) error {
	s.mu.Lock()
	if s.file != nil {
		s.file.Close()
	}
	s.state, s.idx, s.file = cardDataLoading, nil, nil
	cur := s.current()
	os.RemoveAll(cur)
	err := os.Rename(next, cur)
	s.mu.Unlock()
	if err != nil {
		s.mu.Lock()
		s.state, s.err = cardDataError, err.Error()
		s.mu.Unlock()
		return err
	}
	s.load()
	if st := s.status(); st.State != cardDataReady {
		return fmt.Errorf("installing card data: %s", st.Error)
	}
	return nil
}

// lazyReader opens its source on the first read, so the second bulk file is
// requested only once the first has been read through
type lazyReader struct {
	open func() (io.ReadCloser, error)
	rc   io.ReadCloser
	err  error
}

func (l *lazyReader) Read(p []byte) (int, error) {
	if l.rc == nil && l.err == nil {
		l.rc, l.err = l.open()
	}
	if l.err != nil {
		return 0, l.err
	}
	return l.rc.Read(p)
}

func (l *lazyReader) Close() error {
	if l.rc != nil {
		return l.rc.Close()
	}
	return nil
}

// countingReader adds the bytes read to a running total and reports it
type countingReader struct {
	r      io.Reader
	n      *atomic.Int64
	report func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.report(c.n.Add(int64(n)))
	return n, err
}

// readCloser reads from a decompressor and closes the body beneath it
type readCloser struct {
	io.Reader
	body io.Closer
}

func (r readCloser) Close() error { return r.body.Close() }
