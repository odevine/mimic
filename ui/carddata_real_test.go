package main

import (
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLocalCardsRealBulk builds a store from real Scryfall bulk files and checks
// a few lookups against it. It runs only when MIMIC_BULK_DIR names a folder
// holding an oracle-cards and a default-cards .jsonl.gz download
func TestLocalCardsRealBulk(t *testing.T) {
	src := os.Getenv("MIMIC_BULK_DIR")
	if src == "" {
		t.Skip("set MIMIC_BULK_DIR to run against real bulk data")
	}
	open := func(pattern string) *gzip.Reader {
		t.Helper()
		m, _ := filepath.Glob(filepath.Join(src, pattern))
		if len(m) == 0 {
			t.Fatalf("no %s in %s", pattern, src)
		}
		f, err := os.Open(m[0])
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		z, err := gzip.NewReader(f)
		if err != nil {
			t.Fatal(err)
		}
		return z
	}

	dir := t.TempDir()
	start := time.Now()
	meta, err := buildLocalCards(context.Background(), filepath.Join(dir, "current"), open("oracle-cards-*.jsonl.gz"), open("default-cards-*.jsonl.gz"), time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("built %d printings, %.1f MB of records, in %v", meta.Printings, float64(meta.Bytes)/1e6, time.Since(start).Round(time.Millisecond))

	s := newLocalStore(dir)
	start = time.Now()
	s.load()
	if !s.ready() {
		t.Fatalf("store not ready: %+v", s.status())
	}
	t.Logf("loaded the index in %v", time.Since(start).Round(time.Millisecond))

	start = time.Now()
	for _, name := range []string{"Lightning Bolt", "Sol Ring", "Counterspell", "Llanowar Elves", "Swords to Plowshares", "Kykar, Zephyr Awakener", "Lim-Dul's Vault", "Delver of Secrets"} {
		d, err := s.exact(name)
		if err != nil || d == nil {
			t.Errorf("exact %q: %v, %v", name, d, err)
			continue
		}
		t.Logf("exact %-24q -> %s [%s %s] %s", name, d.Name, d.SetCode, d.CollectorNumber, d.ArtworkURL)
	}
	t.Logf("8 exact lookups in %v", time.Since(start).Round(time.Microsecond))

	if d, _ := s.printing("Lightning Bolt", "2x2", "117"); d == nil || d.SetCode != "2x2" {
		t.Errorf("printing 2X2 117 = %+v", d)
	}
	start = time.Now()
	d, _ := s.fuzzy("Lighning Bolt")
	t.Logf("fuzzy Lighning Bolt -> %v in %v", d != nil, time.Since(start).Round(time.Microsecond))
	sim, _ := s.similar("Bolt", 8)
	var names []string
	for _, c := range sim {
		names = append(names, c.Name)
	}
	t.Logf("similar Bolt -> %v", names)
	ps, _ := s.printings("Sol Ring")
	t.Logf("Sol Ring has %d printings, newest %s", len(ps), ps[0].SetCode)
}
