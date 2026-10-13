package fonts

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPoolIndexesByFamilyAndStyle(t *testing.T) {
	dir := t.TempDir()
	// File names and folders mean nothing, only the name table does.
	reg := writeFont(t, dir, "anything.ttf", plantinRegular)
	ital := writeFont(t, dir, "deep/er/z.otf", plantinItalic)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPool(dir)
	if got := len(p.Fonts()); got != 2 {
		t.Fatalf("pool holds %d fonts, want 2", got)
	}
	for _, tc := range []struct{ ask, path, name string }{
		{"Plantin MT Pro Regular", reg, "Plantin MT Pro Regular"},
		{"plantin mt pro italic", ital, "Plantin MT Pro Italic"},
		{"  PLANTIN  MT PRO  ITALIC ", ital, "Plantin MT Pro Italic"},
	} {
		got, ok := p.Lookup(tc.ask)
		if !ok || got.Path != tc.path || got.Name != tc.name {
			t.Errorf("Lookup(%q) = %+v, %v, want %s at %s", tc.ask, got, ok, tc.name, tc.path)
		}
	}
	if _, ok := p.Lookup("Plantin MT Pro Bold"); ok {
		t.Error("Lookup found a font the pool does not hold")
	}
}

func TestPoolReportsDuplicatesAndRejects(t *testing.T) {
	dir := t.TempDir()
	first := writeFont(t, dir, "a.ttf", plantinRegular)
	second := writeFont(t, dir, "b.ttf", plantinRegular)
	bad := filepath.Join(dir, "c.ttf")
	if err := os.WriteFile(bad, []byte("not a font"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPool(dir)
	if got, _ := p.Lookup("Plantin MT Pro Regular"); got.Path != first {
		t.Errorf("duplicate resolved to %s, want the first by path %s", got.Path, first)
	}
	if d := p.Duplicates(); len(d) != 1 || d[0] != second {
		t.Errorf("duplicates = %v, want [%s]", d, second)
	}
	if r := p.Rejected(); len(r) != 1 || r[0].Path != bad || r[0].Err == nil {
		t.Errorf("rejected = %+v, want %s with an error", r, bad)
	}
}

func TestPoolSkipsHiddenAndTolerantOfAbsence(t *testing.T) {
	dir := t.TempDir()
	writeFont(t, dir, ".hidden/a.ttf", plantinRegular)
	writeFont(t, dir, "._junk.ttf", plantinRegular)
	if got := len(NewPool(dir).Fonts()); got != 0 {
		t.Errorf("hidden files were indexed: %d fonts", got)
	}
	for _, p := range []*Pool{nil, NewPool(""), NewPool(filepath.Join(dir, "missing"))} {
		if len(p.Fonts()) != 0 || len(p.Duplicates()) != 0 || len(p.Rejected()) != 0 {
			t.Errorf("an empty pool reported contents: %+v", p)
		}
		if _, ok := p.Lookup("anything"); ok {
			t.Error("an empty pool found a font")
		}
	}
}

func TestReplacedPoolFileIsReparsed(t *testing.T) {
	dir := t.TempDir()
	path := writeFont(t, dir, "Font.ttf", plantinRegular)
	if got := NewPool(dir).Fonts()[0].Style; got != "Regular" {
		t.Fatalf("style = %q, want Regular", got)
	}
	writeFont(t, dir, "Font.ttf", plantinItalic)
	// Some filesystems keep coarse modification times, so force a distinct one
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if got := NewPool(dir).Fonts()[0].Style; got != "Italic" {
		t.Errorf("style after replacing = %q, want Italic", got)
	}
}
