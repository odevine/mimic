package resource

import (
	"runtime"
	"testing"
)

const (
	mib = 1 << 20
	gib = 1 << 30
)

// The model was fitted to measured peaks, so a figure far from them means a
// constant was changed without measuring again
func TestRenderBytesMatchesMeasuredPeaks(t *testing.T) {
	cases := []struct {
		name      string
		w, h      int
		low, high uint64
	}{
		{"300 dpi", 816, 1110, 230 * mib, 330 * mib},
		{"600 dpi", 1632, 2220, 380 * mib, 520 * mib},
		{"authored size", 3264, 4440, 1000 * mib, 1300 * mib},
	}
	for _, c := range cases {
		if got := RenderBytes(c.w, c.h); got < c.low || got > c.high {
			t.Errorf("%s: RenderBytes = %d MiB, want between %d and %d MiB", c.name, got/mib, c.low/mib, c.high/mib)
		}
	}
	if got := RenderBytes(-1, 100); got != renderFixed {
		t.Errorf("RenderBytes with a negative size = %d, want the fixed cost", got)
	}
}

func TestMemoryBudget(t *testing.T) {
	cases := []struct {
		name string
		m    Memory
		want uint64
	}{
		{"free memory is known", Memory{Total: 16 * gib, Available: 10 * gib}, 7 * gib},
		{"only the total is known", Memory{Total: 16 * gib}, 8 * gib},
		{"nothing is known", Memory{}, 0},
	}
	for _, c := range cases {
		if got := c.m.Budget(); got != c.want {
			t.Errorf("%s: Budget = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestWorkers(t *testing.T) {
	medium := CacheBytes(DefaultCacheSize)
	native := RenderBytes(3264, 4440)
	small := RenderBytes(816, 1110)
	cases := []struct {
		name      string
		m         Memory
		perRender uint64
		cpus      int
		want      int
	}{
		{"a small machine fits one full-size render", Memory{Total: 4 * gib, Available: 2 * gib}, native, 8, 1},
		{"memory is the limit", Memory{Total: 16 * gib, Available: 8 * gib}, native, 16, 4},
		{"processors are the limit", Memory{Total: 128 * gib, Available: 100 * gib}, small, 4, 4},
		{"the ceiling is the limit", Memory{Total: 512 * gib, Available: 400 * gib}, small, 128, MaxWorkers},
		{"half the total when free memory is unknown", Memory{Total: 16 * gib}, native, 16, 6},
		{"unknown memory keeps the old default", Memory{}, native, 16, 2},
		{"unknown memory on a one processor machine", Memory{}, native, 1, 1},
		{"no processor count means the ceiling", Memory{Total: 512 * gib, Available: 400 * gib}, small, 0, MaxWorkers},
		{"a budget under the base still runs one", Memory{Total: 1 * gib, Available: 256 * mib}, native, 8, 1},
	}
	for _, c := range cases {
		if got := Workers(c.m, c.perRender, c.cpus, medium); got != c.want {
			t.Errorf("%s: Workers = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestRunBytesGrowsByOneRenderPerWorker(t *testing.T) {
	per := RenderBytes(3264, 4440)
	cache := CacheBytes(DefaultCacheSize)
	if got := RunBytes(3, per, cache) - RunBytes(2, per, cache); got != per {
		t.Errorf("a third worker adds %d bytes, want %d", got, per)
	}
	if RunBytes(0, per, cache) != BaseBytes(cache) || RunBytes(-1, per, cache) != BaseBytes(cache) {
		t.Error("no workers should cost just the base")
	}
}

func TestBaseHoldsTheLayerCache(t *testing.T) {
	cache := CacheBytes("large")
	if BaseBytes(cache) != baseBytes+cache {
		t.Errorf("BaseBytes = %d, want the app's base plus the %d byte cache", BaseBytes(cache), cache)
	}
}

func TestCacheSizes(t *testing.T) {
	want := map[string]uint64{"small": 256 * mib, "medium": 512 * mib, "large": 1 * gib, "xlarge": 2 * gib}
	if len(CacheSizes) != len(want) {
		t.Fatalf("%d sizes, want %d", len(CacheSizes), len(want))
	}
	for _, c := range CacheSizes {
		if c.Label == "" || want[c.Name] != c.Bytes || CacheBytes(c.Name) != c.Bytes || !ValidCacheSize(c.Name) {
			t.Errorf("size %+v is not what the settings promise", c)
		}
	}
	for _, name := range []string{"", "huge", "Medium"} {
		if ValidCacheSize(name) {
			t.Errorf("%q should not be a size", name)
		}
		if CacheBytes(name) != want[DefaultCacheSize] {
			t.Errorf("CacheBytes(%q) = %d, want the default", name, CacheBytes(name))
		}
	}
}

// A bigger cache leaves less memory for renders, so it can only lower the
// worker count
func TestBiggerCacheNeverAddsWorkers(t *testing.T) {
	m := Memory{Total: 16 * gib, Available: 10 * gib}
	per := RenderBytes(3264, 4440)
	prev := MaxWorkers + 1
	for _, c := range CacheSizes {
		n := Workers(m, per, 16, c.Bytes)
		if n > prev {
			t.Errorf("%s cache gives %d workers, more than a smaller one's %d", c.Name, n, prev)
		}
		prev = n
	}
}

func TestReadMemoryOnThisPlatform(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		t.Skip("no memory reader on this platform")
	}
	m := ReadMemory()
	if m.Total < 256*mib {
		t.Errorf("Total = %d bytes, want the installed memory", m.Total)
	}
	if m.Available > m.Total {
		t.Errorf("Available %d exceeds Total %d", m.Available, m.Total)
	}
}
