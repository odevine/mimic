package carddata

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
	"sync/atomic"
	"time"

	"github.com/odevine/mimic/ui/internal/scryfall"
)

// bulkListURL is Scryfall's index of bulk data files. A test points it at a
// local server
var bulkListURL = "https://api.scryfall.com/bulk-data"

// bulkFile is one entry of Scryfall's bulk data index
type bulkFile struct {
	Type      string    `json:"type"`
	UpdatedAt time.Time `json:"updated_at"`
	URI       string    `json:"jsonl_download_uri"`
	Size      int64     `json:"compressed_size"`
}

// Remote is what a download would fetch, as the settings panel shows it
type Remote struct {
	UpdatedAt time.Time `json:"updatedAt"`
	Size      int64     `json:"size"`

	oracle, cards bulkFile
}

// FetchRemote reads Scryfall's bulk index for the two files a local copy is
// built from
func FetchRemote(ctx context.Context, client *http.Client) (*Remote, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bulkListURL, nil)
	if err != nil {
		return nil, err
	}
	scryfall.SetHeaders(req)
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
	out := &Remote{}
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

// Fetch downloads both bulk files info names and builds a new copy from them as
// they stream in, so neither download is ever held whole in memory or on disk.
// The copy is built beside the installed one, for Install to swap in. progress,
// when set, is told the compressed bytes read so far and info.Size
func (s *Store) Fetch(ctx context.Context, client *http.Client, info *Remote, progress func(done, total int64)) (Meta, error) {
	var read atomic.Int64
	counted := func(file bulkFile) *lazyReader {
		return &lazyReader{open: func() (io.ReadCloser, error) {
			body, err := openBulk(ctx, client, file.URI)
			if err != nil {
				return nil, err
			}
			z, err := gzip.NewReader(&countingReader{r: body, n: &read, report: func(n int64) {
				if progress != nil {
					progress(n, info.Size)
				}
			}})
			if err != nil {
				body.Close()
				return nil, fmt.Errorf("reading %s: %w", file.Type, err)
			}
			return readCloser{z, body}, nil
		}}
	}
	oracle, cards := counted(info.oracle), counted(info.cards)
	defer oracle.Close()
	defer cards.Close()

	next := s.next()
	os.RemoveAll(next)
	meta, err := Build(ctx, next, oracle, cards, info.UpdatedAt, nil)
	if err != nil {
		os.RemoveAll(next)
		return Meta{}, err
	}
	return meta, nil
}

// next is where Fetch builds a copy before Install swaps it in
func (s *Store) next() string { return filepath.Join(s.dir, "next") }

// openBulk starts one bulk file download
func openBulk(ctx context.Context, client *http.Client, uri string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	scryfall.SetHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading card data: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("downloading card data: %s", resp.Status)
	}
	return resp.Body, nil
}

// BeginDownload marks a download as running, refusing a second one
func (s *Store) BeginDownload(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobID != "" {
		return false
	}
	s.jobID = id
	return true
}

// EndDownload clears the running download, so Status stops reporting it
func (s *Store) EndDownload() {
	s.mu.Lock()
	s.jobID = ""
	s.mu.Unlock()
}

// Install swaps the copy Fetch built in for the installed one and loads it.
// Readers hold the read lock through a record read, so the old file is never
// closed under one
func (s *Store) Install() error {
	s.mu.Lock()
	if s.file != nil {
		s.file.Close()
	}
	s.state, s.idx, s.file = StateLoading, nil, nil
	cur := s.current()
	os.RemoveAll(cur)
	err := os.Rename(s.next(), cur)
	s.mu.Unlock()
	if err != nil {
		s.mu.Lock()
		s.state, s.err = StateError, err.Error()
		s.mu.Unlock()
		return err
	}
	s.Load()
	if st := s.Status(); st.State != StateReady {
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
