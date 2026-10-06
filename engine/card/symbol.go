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

// SetSymbol is a set's expansion symbol, drawn in its rarity's colors. It comes
// from the mtg-vectors catalog, https://github.com/Investigamer/mtg-vectors,
// whose icons carry their own gradients and outlines
type SetSymbol struct {
	// Code is the catalog folder the symbol was read from, such as "M21"
	Code string
	Icon *svgpath.Icon
}

const (
	// setsIndexName and indexMaxAge name the cached set code to icon code index and
	// how long a copy is trusted. Scryfall adds sets every few weeks, so a week
	// keeps new sets appearing without asking for the list on every launch
	setsIndexName = "sets-icons.json"
	indexMaxAge   = 7 * 24 * time.Hour

	// catalogName, catalogMaxAge and defaultCatalogURL name the cached catalog
	// zip, how long a copy is trusted, and where its latest release is published.
	// The catalog is updated weekly
	catalogName       = "mtg-vectors.optimized.zip"
	catalogMaxAge     = 7 * 24 * time.Hour
	defaultCatalogURL = "https://github.com/Investigamer/mtg-vectors/releases/latest/download/mtg-vectors.optimized.zip"
	maxCatalogBytes   = 64 << 20
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

// WithSymbolCatalogURL overrides where the symbol catalog zip is downloaded
// from, mainly for tests
func WithSymbolCatalogURL(u string) Option { return func(cl *Client) { cl.symbols.catalogURL = u } }

// symbolState is the set symbol data a client holds between calls: the index of
// icon codes, the catalog, and each icon already parsed, so a batch of cards
// from one set reads and parses its symbol once
type symbolState struct {
	cache      SymbolCache
	catalogURL string

	mu        sync.Mutex
	index     map[string]string // lowercase set code to uppercase icon code
	indexedAt time.Time
	catalog   *catalog
	loadedAt  time.Time
	icons     map[string]*SetSymbol // keyed by folder and rarity letter
}

// FetchSetSymbol returns the expansion symbol of the card's set, colored for its
// rarity. It returns nil and no error when there is nothing to draw: the card
// has no set code, Scryfall does not list the set, or the catalog has no symbol
// for the icon Scryfall gives it. Any other failure is an error, so a caller can
// say the symbol is unavailable and render without it
func (c *Client) FetchSetSymbol(ctx context.Context, d *Data) (*SetSymbol, error) {
	code := strings.ToLower(strings.TrimSpace(d.SetCode))
	if code == "" {
		return nil, nil
	}
	iconCode, err := c.setIconCode(ctx, code)
	if err != nil || iconCode == "" {
		return nil, err
	}
	cat, err := c.loadCatalog(ctx)
	if err != nil {
		return nil, err
	}
	folder, ok := cat.folder(code, iconCode)
	if !ok {
		return nil, nil
	}

	key := folder + "/" + strings.Join(rarityLetters(d.Rarity), "")
	c.symbols.mu.Lock()
	sym, ok := c.symbols.icons[key]
	c.symbols.mu.Unlock()
	if ok {
		return sym, nil
	}
	icon, _, found, err := cat.icon(folder, d.Rarity)
	if err != nil {
		return nil, fmt.Errorf("set symbol for %q: %w", code, err)
	}
	if !found {
		return nil, nil
	}
	sym = &SetSymbol{Code: folder, Icon: icon}
	c.symbols.mu.Lock()
	if c.symbols.icons == nil {
		c.symbols.icons = make(map[string]*SetSymbol)
	}
	c.symbols.icons[key] = sym
	c.symbols.mu.Unlock()
	return sym, nil
}

// setIconCode looks a set code up in the icon index, loading the index first
// when none is held or the held one is old. It returns "" for a code the index
// does not list
func (c *Client) setIconCode(ctx context.Context, code string) (string, error) {
	c.symbols.mu.Lock()
	defer c.symbols.mu.Unlock()
	if c.symbols.index == nil || time.Since(c.symbols.indexedAt) > indexMaxAge {
		index, err := c.loadSetsIndex(ctx)
		if err != nil {
			return "", err
		}
		c.symbols.index, c.symbols.indexedAt = index, time.Now()
	}
	return c.symbols.index[code], nil
}

// loadSetsIndex reads the set code to icon code index from the cache, or from
// Scryfall's set list when the cache has none that is fresh. A set's icon code
// is the name of the icon file Scryfall points it at, which for a set with no
// symbol of its own is its parent's. The caller holds the symbol lock, so
// concurrent renders share one request
func (c *Client) loadSetsIndex(ctx context.Context) (map[string]string, error) {
	if cache := c.symbols.cache; cache != nil {
		if raw, ok := cache.Get(setsIndexName, indexMaxAge); ok {
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
		u, err := url.Parse(s.IconSVGURI)
		if err != nil || s.Code == "" {
			continue
		}
		if icon := strings.ToUpper(strings.TrimSuffix(path.Base(u.Path), ".svg")); icon != "" && icon != "." && icon != "/" {
			index[strings.ToLower(s.Code)] = icon
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

// loadCatalog returns the symbol catalog, from memory, the cache, or a download
// of the latest release. A copy older than a week is replaced, and when the
// replacement cannot be downloaded the old copy is used instead of failing
func (c *Client) loadCatalog(ctx context.Context) (*catalog, error) {
	c.symbols.mu.Lock()
	defer c.symbols.mu.Unlock()
	if c.symbols.catalog != nil && time.Since(c.symbols.loadedAt) <= catalogMaxAge {
		return c.symbols.catalog, nil
	}
	cache := c.symbols.cache
	set := func(cat *catalog) *catalog {
		c.symbols.catalog, c.symbols.loadedAt = cat, time.Now()
		c.symbols.icons = nil
		return cat
	}

	if cache != nil {
		if data, ok := cache.Get(catalogName, catalogMaxAge); ok {
			if cat, err := openCatalog(data); err == nil {
				return set(cat), nil
			}
		}
	}
	data, err := c.getBounded(ctx, c.symbols.catalogURL, "application/zip", maxCatalogBytes)
	if err == nil {
		var cat *catalog
		if cat, err = openCatalog(data); err == nil {
			if cache != nil {
				cache.Put(catalogName, data)
			}
			return set(cat), nil
		}
	}
	// A copy that has gone stale still draws the symbols it has
	if c.symbols.catalog != nil {
		return c.symbols.catalog, nil
	}
	if cache != nil {
		if old, ok := cache.Get(catalogName, 0); ok {
			if cat, oerr := openCatalog(old); oerr == nil {
				return set(cat), nil
			}
		}
	}
	return nil, fmt.Errorf("set symbols: downloading the catalog: %w", err)
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
