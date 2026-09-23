package main

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// scryfallAPIHost is the only host the pacer slows. Scryfall's image hosts carry
// no rate limit, so art downloads skip the queue
const scryfallAPIHost = "api.scryfall.com"

// scryfallInterval spaces API requests about ten a second, inside Scryfall's
// guidance of 50 to 100 milliseconds between requests
const scryfallInterval = 100 * time.Millisecond

// maxRetryAfter caps how long a 429 can hold every later request back, so a
// malformed or hostile Retry-After cannot stall the app
const maxRetryAfter = 30 * time.Second

// scryfallHTTPClient is the HTTP client the card client uses. Pacing happens
// inside the transport, so every search, lookup and printings call shares one
// queue. The timeout is a backstop, and callers bound their own requests
func scryfallHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   netTimeout,
		Transport: newPacedTransport(http.DefaultTransport, scryfallAPIHost, scryfallInterval),
	}
}

// pacedTransport spaces requests to one host at least interval apart, however
// many goroutines are calling. A 429 pushes the next slot back by the response's
// Retry-After for every caller, and the request that drew it is retried once
type pacedTransport struct {
	base     http.RoundTripper
	host     string
	interval time.Duration

	mu   sync.Mutex
	next time.Time
}

func newPacedTransport(base http.RoundTripper, host string, interval time.Duration) *pacedTransport {
	return &pacedTransport{base: base, host: host, interval: interval}
}

func (t *pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != t.host {
		return t.base.RoundTrip(req)
	}
	if err := t.wait(req.Context()); err != nil {
		return nil, err
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests || req.Body != nil {
		return resp, err
	}
	t.backOff(retryAfter(resp.Header.Get("Retry-After")))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if err := t.wait(req.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// wait reserves the next free slot and sleeps until it arrives, returning early
// with the context's error if the caller gives up first
func (t *pacedTransport) wait(ctx context.Context) error {
	t.mu.Lock()
	now := time.Now()
	slot := t.next
	if slot.Before(now) {
		slot = now
	}
	t.next = slot.Add(t.interval)
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

// backOff moves the next free slot to at least d from now
func (t *pacedTransport) backOff(d time.Duration) {
	t.mu.Lock()
	if until := time.Now().Add(d); until.After(t.next) {
		t.next = until
	}
	t.mu.Unlock()
}

// retryAfter reads a Retry-After header given in seconds, defaulting to one
// second when it is missing or unreadable and capping it at maxRetryAfter
func retryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return time.Second
	}
	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}
