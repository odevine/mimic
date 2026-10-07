package template

import (
	"errors"
	"image"
	"io"
	"slices"
	"sync"

	"github.com/odevine/impasto/blend"
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
	// index is built the first time the layer is asked for with one, and goes
	// when the layer does. It is small beside the pixels, so it is not counted
	// against the budget
	index   sync.Once
	indexed *blend.Indexed
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
// pixels. A budget of zero or less caches nothing. SetBudget changes it later
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
	c.mu.Lock()
	if c.budget <= 0 {
		c.mu.Unlock()
		return decodeImage(c.AssetProvider, relPath)
	}
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

// LoadIndexed is LoadImage for a layer that is only composited. A resident
// *image.NRGBA comes back as a *blend.Indexed, whose index of visible pixels is
// built once, the first time it is asked for, and kept as long as the layer is
// resident. A layer that is not cached, because it did not fit or the cache is
// off, or that is not an NRGBA image, comes back as LoadImage gives it
func (c *CachedAssets) LoadIndexed(relPath string) (image.Image, error) {
	img, err := c.LoadImage(relPath)
	if err != nil {
		return nil, err
	}
	n, ok := img.(*image.NRGBA)
	if !ok {
		return img, nil
	}
	c.mu.Lock()
	e, resident := c.resident[relPath]
	c.mu.Unlock()
	if !resident || e.img != image.Image(n) {
		return img, nil
	}
	e.index.Do(func() { e.indexed = blend.Index(n) })
	return e.indexed, nil
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
		order := c.coldFirst()
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

// coldFirst lists the resident layers from the least used to the most, with the
// least recently loaded first among layers used equally
func (c *CachedAssets) coldFirst() []string {
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
	return order
}

// SetBudget changes how much decoded pixel data the cache may hold. A smaller
// budget evicts the least used layers until what remains fits, and a budget of
// zero or less empties the cache and stops caching. Images already returned to
// callers stay valid
func (c *CachedAssets) SetBudget(budget int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.budget = budget
	if c.used <= budget {
		return
	}
	for _, k := range c.coldFirst() {
		c.used -= c.resident[k].bytes
		delete(c.resident, k)
		if c.used <= max(budget, 0) {
			break
		}
	}
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
