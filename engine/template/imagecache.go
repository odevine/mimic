package template

import (
	"errors"
	"image"
	"io"
	"slices"
	"sync"
)

// DefaultImageCacheBytes is the size a caller can pass NewCachedAssets when it
// has no better figure. Frame layers are document-sized, about 58 MB each once
// decoded at full size, so this holds about nine
const DefaultImageCacheBytes = 512 << 20

// agePeriod is how many loads pass between halvings of every layer's use count,
// so a layer that was busy in an earlier batch stops outranking a newer one
const agePeriod = 512

// CachedAssets wraps an AssetProvider and keeps decoded layer images in memory,
// so a batch of cards that share frame layers decodes each PNG once rather than
// once per card. LoadImage consults it automatically. The images it returns are
// shared, so callers must not write to them
//
// Layers are kept by how often they are loaded. A load that would push the
// cache over its budget only displaces layers loaded no more often than itself,
// which keeps the layers nearly every card uses resident while a long tail of
// one-off layers passes through. Loads of the same layer that overlap share one
// decode. It is safe for concurrent use
type CachedAssets struct {
	AssetProvider
	budget int64

	mu       sync.Mutex
	resident map[string]*cachedImage
	inflight map[string]*decoding
	uses     map[string]int
	loads    int
	used     int64
	tick     uint64
	hits     int64
	misses   int64
}

type cachedImage struct {
	img   image.Image
	bytes int64
	last  uint64
}

type decoding struct {
	done chan struct{}
	img  image.Image
	err  error
}

// CacheStats describes a CachedAssets. Hits are loads served without a decode,
// including loads that waited on another's decode
type CacheStats struct {
	Hits, Misses int64
	Layers       int
	Bytes        int64
}

// NewCachedAssets wraps p with a cache of at most budget bytes of decoded
// pixels. A budget of zero or less caches nothing
func NewCachedAssets(p AssetProvider, budget int64) *CachedAssets {
	return &CachedAssets{
		AssetProvider: p,
		budget:        budget,
		resident:      map[string]*cachedImage{},
		inflight:      map[string]*decoding{},
		uses:          map[string]int{},
	}
}

// LoadImage returns the decoded layer at relPath, decoding it only when no
// other load has
func (c *CachedAssets) LoadImage(relPath string) (image.Image, error) {
	if c.budget <= 0 {
		return decodeImage(c.AssetProvider, relPath)
	}
	c.mu.Lock()
	c.count(relPath)
	if e, ok := c.resident[relPath]; ok {
		c.hits++
		c.tick++
		e.last = c.tick
		c.mu.Unlock()
		return e.img, nil
	}
	if d, ok := c.inflight[relPath]; ok {
		c.hits++
		c.mu.Unlock()
		<-d.done
		return d.img, d.err
	}
	d := &decoding{done: make(chan struct{}), err: errors.New("template: layer decode did not finish")}
	c.inflight[relPath] = d
	c.misses++
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.inflight, relPath)
		if d.err == nil {
			c.admit(relPath, d.img)
		}
		c.mu.Unlock()
		close(d.done)
	}()
	d.img, d.err = decodeImage(c.AssetProvider, relPath)
	return d.img, d.err
}

// count records a load of path and ages every count once per agePeriod loads
func (c *CachedAssets) count(path string) {
	c.uses[path]++
	c.loads++
	if c.loads%agePeriod != 0 {
		return
	}
	for k, n := range c.uses {
		if n /= 2; n == 0 {
			delete(c.uses, k)
		} else {
			c.uses[k] = n
		}
	}
}

// admit keeps img if it fits, evicting the least used layers to make room. It
// declines when making room would evict a layer used more often than path
func (c *CachedAssets) admit(path string, img image.Image) {
	size := imageBytes(img)
	if size > c.budget {
		return
	}
	if need := c.used + size - c.budget; need > 0 {
		order := make([]string, 0, len(c.resident))
		for k := range c.resident {
			order = append(order, k)
		}
		slices.SortFunc(order, func(a, b string) int {
			if d := c.uses[a] - c.uses[b]; d != 0 {
				return d
			}
			return int(int64(c.resident[a].last) - int64(c.resident[b].last))
		})
		var freed int64
		n := 0
		for _, k := range order {
			if c.uses[k] > c.uses[path] {
				return
			}
			freed += c.resident[k].bytes
			n++
			if freed >= need {
				break
			}
		}
		if freed < need {
			return
		}
		for _, k := range order[:n] {
			c.used -= c.resident[k].bytes
			delete(c.resident, k)
		}
	}
	c.tick++
	c.resident[path] = &cachedImage{img: img, bytes: size, last: c.tick}
	c.used += size
}

// Stats reports what the cache has served and holds
func (c *CachedAssets) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{Hits: c.hits, Misses: c.misses, Layers: len(c.resident), Bytes: c.used}
}

// Close drops every cached image and closes the wrapped provider if it can be
// closed
func (c *CachedAssets) Close() error {
	c.mu.Lock()
	c.resident = map[string]*cachedImage{}
	c.used = 0
	c.mu.Unlock()
	if cl, ok := c.AssetProvider.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}

// imageBytes is the memory an image's pixels occupy
func imageBytes(img image.Image) int64 {
	switch m := img.(type) {
	case *image.NRGBA:
		return int64(len(m.Pix))
	case *image.RGBA:
		return int64(len(m.Pix))
	case *image.NRGBA64:
		return int64(len(m.Pix))
	case *image.Gray:
		return int64(len(m.Pix))
	case *image.Gray16:
		return int64(len(m.Pix))
	case *image.Paletted:
		return int64(len(m.Pix))
	}
	b := img.Bounds()
	return int64(b.Dx()) * int64(b.Dy()) * 8
}
