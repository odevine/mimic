// Package scryfall is the HTTP layer the UI uses to reach Scryfall: a client
// that paces requests to the published rate limits and the headers Scryfall
// asks every client to send
package scryfall

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// apiHost is the only host the pacer slows. Scryfall's image hosts carry no
// rate limit, so art downloads skip the queue
const apiHost = "api.scryfall.com"

// Scryfall's published limits: search, named, random and collection allow two
// requests a second, and every other API method ten. Each has its own queue, so
// a burst of searches never holds back a cheaper call. The limits are listed at
// https://scryfall.com/docs/api/rate-limits
const (
	defaultInterval = 100 * time.Millisecond
	slowInterval    = 500 * time.Millisecond
)

var slowPaths = []string{"/cards/search", "/cards/named", "/cards/random", "/cards/collection"}

// penalty is how long Scryfall limits access after a 429, and what a 429
// without a Retry-After header is taken to mean
const penalty = 30 * time.Second

// maxRetryAfter caps how long a 429 can hold every later request back, so a
// malformed or hostile Retry-After cannot stall the app
const maxRetryAfter = 2 * penalty

// NewHTTPClient returns the HTTP client the card client uses. Pacing happens inside
// the transport, so every search, lookup and printings call shares one set of
// queues. The timeout applies to each request once it leaves the queue, since
// time spent waiting its turn is not the network being slow
func NewHTTPClient(timeout time.Duration) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.ResponseHeaderTimeout = timeout
	t := newPacedTransport(base, apiHost, defaultInterval)
	for _, p := range slowPaths {
		t.slow[p] = slowInterval
	}
	return &http.Client{Transport: t}
}

// pacedTransport spaces requests to one host, however many goroutines are
// calling: each path in slow waits its own interval, and every other path
// shares the default one. A 429 holds every queue back by the response's
// Retry-After, and the request that drew it is retried once
type pacedTransport struct {
	base     http.RoundTripper
	host     string
	interval time.Duration
	slow     map[string]time.Duration

	mu   sync.Mutex
	next map[string]time.Time
}

func newPacedTransport(base http.RoundTripper, host string, interval time.Duration) *pacedTransport {
	return &pacedTransport{
		base:     base,
		host:     host,
		interval: interval,
		slow:     make(map[string]time.Duration),
		next:     make(map[string]time.Time),
	}
}

func (t *pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != t.host {
		return t.base.RoundTrip(req)
	}
	queue, interval := t.queueFor(req.URL.Path)
	if err := t.wait(req.Context(), queue, interval); err != nil {
		return nil, err
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests || req.Body != nil {
		return resp, err
	}
	t.backOff(retryAfter(resp.Header.Get("Retry-After")))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if err := t.wait(req.Context(), queue, interval); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// queueFor names the queue a path waits in and that queue's interval
func (t *pacedTransport) queueFor(path string) (string, time.Duration) {
	for p, d := range t.slow {
		if path == p || strings.HasPrefix(path, p+"/") {
			return p, d
		}
	}
	return "", t.interval
}

// wait reserves the next free slot in a queue and sleeps until it arrives,
// returning early with the context's error if the caller gives up first
func (t *pacedTransport) wait(ctx context.Context, queue string, interval time.Duration) error {
	t.mu.Lock()
	now := time.Now()
	slot := t.next[queue]
	if slot.Before(now) {
		slot = now
	}
	t.next[queue] = slot.Add(interval)
	t.mu.Unlock()

	d := time.Until(slot)
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// backOff moves every queue's next free slot to at least d from now, since a
// 429 limits the whole client rather than one endpoint
func (t *pacedTransport) backOff(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	until := time.Now().Add(d)
	for _, q := range append([]string{""}, keys(t.slow)...) {
		if until.After(t.next[q]) {
			t.next[q] = until
		}
	}
}

func keys(m map[string]time.Duration) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// retryAfter reads a Retry-After header given in seconds. A missing or
// unreadable one means Scryfall's standard penalty, and the result is capped at
// maxRetryAfter
func retryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return penalty
	}
	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}

// SetHeaders sets the headers Scryfall asks every client to send
func SetHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "mimic (+https://github.com/odevine/mimic)")
	req.Header.Set("Accept", "application/json")
}
