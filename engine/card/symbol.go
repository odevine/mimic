package card

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/odevine/mimic/engine/card/svgpath"
)

// SetSymbol is a set's expansion symbol, the outline Scryfall publishes for it
// as an SVG icon. The icon carries no colour, so a template paints it
type SetSymbol struct {
	// Code is the set code the symbol belongs to, lowercase as Scryfall spells it
	Code string
	Icon *svgpath.Icon
}

const (
	// setsIndexName and setsIndexMaxAge name the cached code to icon URL index
	// and how long a copy is trusted. Scryfall adds sets every few weeks and
	// re-versions an icon's URL when its art changes, so a week keeps new sets
	// appearing without asking for the list on every launch
	setsIndexName   = "sets-index.json"
	setsIndexMaxAge = 7 * 24 * time.Hour

	// defaultIcon is the file name Scryfall points a set at when it has no
	// symbol of its own
	defaultIcon = "default.svg"
)

// SymbolCache keeps the downloads FetchSetSymbol makes, so a later run does not
// repeat them. Both methods are best effort: a miss or a failed write only
// means the next call fetches again
type SymbolCache interface {
	// Get returns the bytes stored under name, or false for a missing entry or
	// one older than maxAge. A zero maxAge accepts an entry of any age
	Get(name string, maxAge time.Duration) ([]byte, bool)
	// Put stores data under name
	Put(name string, data []byte)
}

// WithSymbolCache has the client keep set symbol downloads in c
func WithSymbolCache(c SymbolCache) Option { return func(cl *Client) { cl.symbols.cache = c } }

// symbolState is the set symbol data a client holds between calls: the index of
// icon URLs and each icon already parsed, so a batch of cards from one set
// downloads and parses its symbol once
type symbolState struct {
	cache SymbolCache

	mu        sync.Mutex
	index     map[string]string // set code to icon URL
	indexedAt time.Time
	icons     map[string]*SetSymbol // keyed by icon URL
}

// FetchSetSymbol returns the expansion symbol of the card's set. It returns nil
// and no error when there is nothing to draw: the card has no set code,
// Scryfall does not list the set, or Scryfall lists it with its placeholder
// icon. Any other failure is an error, so a caller can say the symbol is
// unavailable and render without it
func (c *Client) FetchSetSymbol(ctx context.Context, d *Data) (*SetSymbol, error) {
	code := strings.ToLower(strings.TrimSpace(d.SetCode))
	if code == "" {
		return nil, nil
	}
	iconURL, err := c.setIconURL(ctx, code)
	if err != nil || iconURL == "" {
		return nil, err
	}
	u, err := url.Parse(iconURL)
	if err != nil {
		return nil, fmt.Errorf("scryfall: icon URL for set %q: %w", code, err)
	}
	if path.Base(u.Path) == defaultIcon {
		return nil, nil
	}

	c.symbols.mu.Lock()
	sym, ok := c.symbols.icons[iconURL]
	c.symbols.mu.Unlock()
	if ok {
		return sym, nil
	}

	svg, err := c.iconBytes(ctx, code, u)
	if err != nil {
		return nil, err
	}
	icon, err := svgpath.Parse(svg)
	if err != nil {
		return nil, fmt.Errorf("scryfall: set symbol for %q: %w", code, err)
	}
	sym = &SetSymbol{Code: code, Icon: icon}
	c.symbols.mu.Lock()
	if c.symbols.icons == nil {
		c.symbols.icons = make(map[string]*SetSymbol)
	}
	c.symbols.icons[iconURL] = sym
	c.symbols.mu.Unlock()
	return sym, nil
}

// setIconURL looks a set code up in the icon index, loading the index first
// when none is held or the held one is old. It returns "" for a code the index
// does not list
func (c *Client) setIconURL(ctx context.Context, code string) (string, error) {
	c.symbols.mu.Lock()
	defer c.symbols.mu.Unlock()
	if c.symbols.index == nil || time.Since(c.symbols.indexedAt) > setsIndexMaxAge {
		index, err := c.loadSetsIndex(ctx)
		if err != nil {
			return "", err
		}
		c.symbols.index, c.symbols.indexedAt = index, time.Now()
	}
	return c.symbols.index[code], nil
}

// loadSetsIndex reads the code to icon URL index from the cache, or from
// Scryfall's set list when the cache has none that is fresh. The caller holds
// the symbol lock, so concurrent renders share one request
func (c *Client) loadSetsIndex(ctx context.Context) (map[string]string, error) {
	if cache := c.symbols.cache; cache != nil {
		if raw, ok := cache.Get(setsIndexName, setsIndexMaxAge); ok {
			var index map[string]string
			if json.Unmarshal(raw, &index) == nil && len(index) > 0 {
				return index, nil
			}
		}
	}

	body, err := c.getBounded(ctx, c.baseURL+"/sets", "application/json", c.maxListBytes)
	if err != nil {
		return nil, fmt.Errorf("scryfall: fetching set list: %w", err)
	}
	var list struct {
		Data []struct {
			Code       string `json:"code"`
			IconSVGURI string `json:"icon_svg_uri"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("scryfall: decoding set list: %w", err)
	}
	index := make(map[string]string, len(list.Data))
	for _, s := range list.Data {
		if s.Code != "" && s.IconSVGURI != "" {
			index[strings.ToLower(s.Code)] = s.IconSVGURI
		}
	}
	if len(index) == 0 {
		return nil, fmt.Errorf("scryfall: set list holds no icons")
	}
	if cache := c.symbols.cache; cache != nil {
		if raw, err := json.Marshal(index); err == nil {
			cache.Put(setsIndexName, raw)
		}
	}
	return index, nil
}

// iconBytes returns a set's SVG from the cache or from Scryfall. The cache entry
// is named for the icon URL's version query as well as the set, so an icon
// Scryfall re-versions is downloaded again
func (c *Client) iconBytes(ctx context.Context, code string, u *url.URL) ([]byte, error) {
	name := "icon-" + code + "-" + versionToken(u.RawQuery) + ".svg"
	if cache := c.symbols.cache; cache != nil {
		if svg, ok := cache.Get(name, 0); ok {
			return svg, nil
		}
	}
	svg, err := c.getBounded(ctx, u.String(), "image/svg+xml", svgpath.MaxBytes)
	if err != nil {
		return nil, fmt.Errorf("scryfall: fetching symbol for set %q: %w", code, err)
	}
	if cache := c.symbols.cache; cache != nil {
		cache.Put(name, svg)
	}
	return svg, nil
}

// versionToken reduces an icon URL's query, a number Scryfall bumps when the
// icon changes, to characters that are safe in a file name
func versionToken(query string) string {
	var b strings.Builder
	for _, r := range query {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "0"
	}
	return b.String()
}

// getBounded fetches a URL and returns its body, failing on a status other than
// 200 or a body over max bytes
func (c *Client) getBounded(ctx context.Context, rawURL, accept string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	return readBounded(resp.Body, max)
}

// DirCache is a SymbolCache that keeps each entry as a file in one directory.
// The zero value, and one made with an empty directory, caches nothing
type DirCache struct{ dir string }

// NewDirCache returns a cache that stores its entries in dir, creating the
// directory on the first write
func NewDirCache(dir string) *DirCache { return &DirCache{dir: dir} }

// Get reads an entry, missing when the file is absent or older than maxAge
func (d *DirCache) Get(name string, maxAge time.Duration) ([]byte, bool) {
	file, ok := d.file(name)
	if !ok {
		return nil, false
	}
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
		return nil, false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, false
	}
	return data, true
}

// Put writes an entry through a temporary file, so a reader never sees half of
// one. A write that fails is dropped
func (d *DirCache) Put(name string, data []byte) {
	file, ok := d.file(name)
	if !ok {
		return
	}
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(d.dir, ".tmp-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), file) != nil {
		os.Remove(tmp.Name())
	}
}

// file is the path of an entry, false when the cache has no directory or the
// name is not a plain file name
func (d *DirCache) file(name string) (string, bool) {
	if d == nil || d.dir == "" || name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", false
	}
	return filepath.Join(d.dir, name), true
}
