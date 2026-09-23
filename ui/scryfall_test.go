package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// pacedClient returns a client whose transport paces requests to srv
func pacedClient(t *testing.T, srv *httptest.Server, interval time.Duration) *http.Client {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: newPacedTransport(http.DefaultTransport, u.Host, interval)}
}

func TestPacedTransportSpacesConcurrentRequests(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
	}))
	defer srv.Close()

	const interval = 30 * time.Millisecond
	c := pacedClient(t, srv, interval)
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			resp, err := c.Get(srv.URL)
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
		})
	}
	wg.Wait()

	if span := times[len(times)-1].Sub(times[0]); span < 4*interval-5*time.Millisecond {
		t.Errorf("5 requests spanned %v, want at least %v", span, 4*interval)
	}
}

func TestPacedTransportRetriesOnce429(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := pacedClient(t, srv, time.Millisecond).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || hits.Load() != 2 {
		t.Errorf("status %d after %d hits, want 200 after 2", resp.StatusCode, hits.Load())
	}
}

func TestPacedTransportHonorsCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := pacedClient(t, srv, time.Hour)

	// The first request takes the free slot, so the second waits an hour
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if _, err := c.Do(req); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
}

func TestPacedTransportSkipsOtherHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := &http.Client{Transport: newPacedTransport(http.DefaultTransport, scryfallAPIHost, time.Hour)}
	for range 3 {
		resp, err := c.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
}

func TestRetryAfter(t *testing.T) {
	cases := map[string]time.Duration{
		"":       scryfallPenalty,
		"junk":   scryfallPenalty,
		"-3":     scryfallPenalty,
		"0":      0,
		"2":      2 * time.Second,
		"999999": maxRetryAfter,
	}
	for in, want := range cases {
		if got := retryAfter(in); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPacedTransportQueuesSlowPathsApart(t *testing.T) {
	var mu sync.Mutex
	hits := make(map[string][]time.Time)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path] = append(hits[r.URL.Path], time.Now())
		mu.Unlock()
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	tr := newPacedTransport(http.DefaultTransport, u.Host, time.Millisecond)
	tr.slow["/cards/search"] = 60 * time.Millisecond
	c := &http.Client{Transport: tr}

	start := time.Now()
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if resp, err := c.Get(srv.URL + "/cards/search?q=x"); err == nil {
				resp.Body.Close()
			}
		})
	}
	// A cheap call is not stuck behind the searches
	resp, err := c.Get(srv.URL + "/sets")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("a call on the default queue waited %v behind searches", d)
	}
	wg.Wait()

	s := hits["/cards/search"]
	if len(s) != 3 || s[2].Sub(s[0]) < 110*time.Millisecond {
		t.Errorf("3 searches spanned %v, want at least 120ms", s[2].Sub(s[0]))
	}
}
