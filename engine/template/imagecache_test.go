package template

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"sync"
	"sync/atomic"
	"testing"
)

// memAssets serves PNGs from memory and counts how many times each is opened
type memAssets struct {
	files  map[string][]byte
	opens  atomic.Int64
	closed bool
}

func (m *memAssets) Manifest() (*Manifest, error) { return &Manifest{}, nil }

func (m *memAssets) Open(p string) (io.ReadCloser, error) {
	m.opens.Add(1)
	b, ok := m.files[p]
	if !ok {
		return nil, errors.New("missing " + p)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *memAssets) Close() error { m.closed = true; return nil }

// square is a w by w NRGBA, 4*w*w bytes once decoded
func square(t *testing.T, w int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, w))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func newMem(t *testing.T, names ...string) *memAssets {
	m := &memAssets{files: map[string][]byte{}}
	for _, n := range names {
		m.files[n] = square(t, 10)
	}
	return m
}

const layerBytes = 10 * 10 * 4

func mustLoad(t *testing.T, p AssetProvider, name string) image.Image {
	t.Helper()
	img, err := LoadImage(p, name)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestCachedAssetsDecodesOnce(t *testing.T) {
	m := newMem(t, "a")
	c := NewCachedAssets(m, 10*layerBytes)
	first := mustLoad(t, c, "a")
	second := mustLoad(t, c, "a")
	if first != second {
		t.Error("the second load returned a different image")
	}
	if got := m.opens.Load(); got != 1 {
		t.Errorf("opens = %d, want 1", got)
	}
	if s := c.Stats(); s.Hits != 1 || s.Misses != 1 || s.Layers != 1 || s.Bytes != layerBytes {
		t.Errorf("stats = %+v", s)
	}
}

func TestCachedAssetsSharesOverlappingDecodes(t *testing.T) {
	m := newMem(t, "a")
	c := NewCachedAssets(m, 10*layerBytes)
	var wg sync.WaitGroup
	imgs := make([]image.Image, 32)
	for i := range imgs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			imgs[i] = mustLoad(t, c, "a")
		}()
	}
	wg.Wait()
	if got := m.opens.Load(); got != 1 {
		t.Errorf("opens = %d, want 1", got)
	}
	for _, img := range imgs {
		if img != imgs[0] {
			t.Fatal("loads returned different images")
		}
	}
}

func TestCachedAssetsKeepsTheMostUsedLayers(t *testing.T) {
	m := newMem(t, "a", "b", "c")
	c := NewCachedAssets(m, 2*layerBytes)
	for range 5 {
		mustLoad(t, c, "a")
		mustLoad(t, c, "b")
	}
	opens := m.opens.Load()
	// A layer seen once cannot displace layers used five times
	mustLoad(t, c, "c")
	mustLoad(t, c, "a")
	mustLoad(t, c, "b")
	if got := m.opens.Load() - opens; got != 1 {
		t.Errorf("opens after a one-off layer = %d, want 1 (the one-off itself)", got)
	}
}

func TestCachedAssetsEvictsTheLeastUsed(t *testing.T) {
	m := newMem(t, "a", "b", "c")
	c := NewCachedAssets(m, 2*layerBytes)
	for range 3 {
		mustLoad(t, c, "a")
	}
	mustLoad(t, c, "b")
	// c has been used as often as b, so it takes b's place and a stays
	mustLoad(t, c, "c")
	opens := m.opens.Load()
	mustLoad(t, c, "a")
	mustLoad(t, c, "c")
	if got := m.opens.Load() - opens; got != 0 {
		t.Errorf("a and c should be resident, %d opens", got)
	}
	mustLoad(t, c, "b")
	if got := m.opens.Load() - opens; got != 1 {
		t.Errorf("b should have been evicted, %d opens", got)
	}
}

func TestCachedAssetsAgesOutOldFavorites(t *testing.T) {
	m := newMem(t, "old", "new")
	c := NewCachedAssets(m, layerBytes)
	for range 2000 {
		mustLoad(t, c, "old")
	}
	for range 20 * agePeriod {
		mustLoad(t, c, "new")
	}
	opens := m.opens.Load()
	mustLoad(t, c, "new")
	if m.opens.Load() != opens {
		t.Error("the newer layer never displaced the layer that stopped being used")
	}
}

func TestCachedAssetsSkipsWhatCannotFit(t *testing.T) {
	m := newMem(t, "a")
	c := NewCachedAssets(m, layerBytes-1)
	mustLoad(t, c, "a")
	mustLoad(t, c, "a")
	if got := m.opens.Load(); got != 2 {
		t.Errorf("opens = %d, want 2 for a layer over the budget", got)
	}
	if s := c.Stats(); s.Layers != 0 || s.Bytes != 0 {
		t.Errorf("stats = %+v, want empty", s)
	}
}

func TestCachedAssetsWithoutBudgetPassesThrough(t *testing.T) {
	m := newMem(t, "a")
	c := NewCachedAssets(m, 0)
	mustLoad(t, c, "a")
	mustLoad(t, c, "a")
	if got := m.opens.Load(); got != 2 {
		t.Errorf("opens = %d, want 2", got)
	}
}

func TestCachedAssetsDoesNotCacheErrors(t *testing.T) {
	m := newMem(t)
	c := NewCachedAssets(m, 10*layerBytes)
	for range 2 {
		if _, err := LoadImage(c, "nope"); err == nil {
			t.Fatal("want an error for a missing layer")
		}
	}
	if got := m.opens.Load(); got != 2 {
		t.Errorf("opens = %d, want 2", got)
	}
	m.files["nope"] = square(t, 10)
	mustLoad(t, c, "nope")
}

func TestCachedAssetsCloseClosesTheProvider(t *testing.T) {
	m := newMem(t, "a")
	c := NewCachedAssets(m, 10*layerBytes)
	mustLoad(t, c, "a")
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if !m.closed {
		t.Error("the wrapped provider was not closed")
	}
	if s := c.Stats(); s.Layers != 0 || s.Bytes != 0 {
		t.Errorf("stats after Close = %+v", s)
	}
}

// A provider without the cache decodes on every load
func TestLoadImageWithoutCache(t *testing.T) {
	m := newMem(t, "a")
	mustLoad(t, m, "a")
	mustLoad(t, m, "a")
	if got := m.opens.Load(); got != 2 {
		t.Errorf("opens = %d, want 2", got)
	}
}
