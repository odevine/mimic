package card

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"time"

	// Register decoders for the art formats Scryfall serves (art_crop is JPEG)
	_ "image/jpeg"
	_ "image/png"
)

// Resource-exhaustion guards live at this boundary because a network response
// is this module's untrusted input. A slow endpoint cannot stall a render
// (bounded timeout), and a misbehaving one cannot force an unbounded read
// (bounded body) before json.Decode or image.Decode ever sees the bytes
const (
	defaultTimeout      = 15 * time.Second
	defaultMaxCardBytes = 1 << 20  // 1MB, real card JSON is a few KB
	defaultMaxArtBytes  = 64 << 20 // 64MB, a real card-art image is a few MB
	defaultBaseURL      = "https://api.scryfall.com"
	userAgent           = "mimic/0.1 (+https://github.com/odevine/mimic)"
)

// Client is a Scryfall client. The zero value is not usable, call NewClient
type Client struct {
	httpClient   *http.Client
	baseURL      string
	maxCardBytes int64
	maxArtBytes  int64
}

// Option configures a Client
type Option func(*Client)

// WithHTTPClient overrides the underlying HTTP client, including its timeout
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithBaseURL overrides the Scryfall base URL, mainly for tests
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithMaxCardBytes overrides the bound on a card JSON response
func WithMaxCardBytes(n int64) Option { return func(c *Client) { c.maxCardBytes = n } }

// WithMaxArtBytes overrides the bound on an art download
func WithMaxArtBytes(n int64) Option { return func(c *Client) { c.maxArtBytes = n } }

// NewClient returns a client with default settings that options override
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient:   &http.Client{Timeout: defaultTimeout},
		baseURL:      defaultBaseURL,
		maxCardBytes: defaultMaxCardBytes,
		maxArtBytes:  defaultMaxArtBytes,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// FetchByName looks up a card by fuzzy (typo-tolerant) name and maps the
// response into *Data
func (c *Client) FetchByName(ctx context.Context, name string) (*Data, error) {
	endpoint := c.baseURL + "/cards/named?fuzzy=" + url.QueryEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scryfall: fetching %q: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scryfall: fetching %q: unexpected status %s", name, resp.Status)
	}

	body, err := readBounded(resp.Body, c.maxCardBytes)
	if err != nil {
		return nil, fmt.Errorf("scryfall: reading card %q: %w", name, err)
	}
	var sc scryfallCard
	if err := json.Unmarshal(body, &sc); err != nil {
		return nil, fmt.Errorf("scryfall: decoding card %q: %w", name, err)
	}
	return sc.toData(), nil
}

// FetchArt downloads and decodes the card's art_crop image. It is separate
// from FetchByName so a caller that already has the art can skip the network
// round trip
func (c *Client) FetchArt(ctx context.Context, d *Data) (image.Image, error) {
	if d.ArtworkURL == "" {
		return nil, fmt.Errorf("scryfall: card %q has no artwork URL", d.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.ArtworkURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scryfall: fetching art for %q: %w", d.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scryfall: fetching art for %q: unexpected status %s", d.Name, resp.Status)
	}

	body, err := readBounded(resp.Body, c.maxArtBytes)
	if err != nil {
		return nil, fmt.Errorf("scryfall: reading art for %q: %w", d.Name, err)
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("scryfall: decoding art for %q: %w", d.Name, err)
	}
	return img, nil
}

// readBounded reads at most max bytes and reports an error if the source has
// more, so a truncated read never reaches a decoder as if it were complete
func readBounded(r io.Reader, max int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("response exceeds %d byte limit", max)
	}
	return body, nil
}
