package fontdir

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// realFont reads one of the engine's embedded fonts from the checkout, since a
// font has to parse to be accepted
func realFont(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "engine", "fonts", "embedded", "body", name))
	if err != nil {
		t.Skipf("engine fonts not in this checkout: %v", err)
	}
	return raw
}

func TestResolvePrecedence(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	managed := filepath.Join(tmp, "config", "fonts")

	if got := Resolve("", managed); got.Kind != KindManaged || got.Path != managed {
		t.Errorf("no flag, no checkout = %+v, want the managed folder", got)
	}
	if got := Resolve("", ""); got.Kind != KindNone {
		t.Errorf("no config directory = %+v, want none", got)
	}
	if err := os.Mkdir("local-fonts", 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Resolve("", managed); got.Kind != KindCheckout {
		t.Errorf("with local-fonts = %+v, want the checkout", got)
	}
	if got := Resolve(filepath.Join(tmp, "missing"), managed); got.Kind != KindCheckout {
		t.Errorf("missing flag folder = %+v, want it skipped for the checkout", got)
	}
	flag := filepath.Join(tmp, "mine")
	if err := os.Mkdir(flag, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(flag, managed); got.Kind != KindFlag || got.Path != flag {
		t.Errorf("flag = %+v, want the flag folder", got)
	}
}

func TestAddReplacesAndRemoveClears(t *testing.T) {
	d := Dir{Path: t.TempDir(), Kind: KindManaged}
	first, err := d.Add("body", "First.ttf", bytes.NewReader(realFont(t, "Merriweather-Regular.ttf")))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	second, err := d.Add("body", "../Second.ttf", bytes.NewReader(realFont(t, "Merriweather-Italic.ttf")))
	if err != nil {
		t.Fatalf("Add again: %v", err)
	}
	if filepath.Dir(second) != filepath.Join(d.Path, "body") {
		t.Errorf("stored at %s, want inside the body folder", second)
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("first font still present after a replace: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(d.Path, "body"))
	if len(entries) != 1 {
		t.Errorf("body folder holds %d entries, want 1", len(entries))
	}

	if err := d.Remove("body"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(second); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("font still present after Remove: %v", err)
	}
	if err := d.Remove("title"); err != nil {
		t.Errorf("removing an empty role: %v", err)
	}
}

func TestAddRejects(t *testing.T) {
	d := Dir{Path: t.TempDir(), Kind: KindManaged}
	good := realFont(t, "Merriweather-Regular.ttf")
	cases := []struct {
		name, folder, file string
		raw                []byte
	}{
		{"not a font", "body", "x.ttf", []byte("not a font")},
		{"wrong extension", "body", "x.woff2", good},
		{"unknown role", "glyphs", "x.ttf", good},
	}
	for _, tc := range cases {
		if _, err := d.Add(tc.folder, tc.file, bytes.NewReader(tc.raw)); err == nil {
			t.Errorf("%s: Add accepted it", tc.name)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(d.Path, "body")); len(entries) != 0 {
		t.Errorf("a rejected upload left %d files behind", len(entries))
	}

	ro := Dir{Path: t.TempDir(), Kind: KindCheckout}
	if _, err := ro.Add("body", "x.ttf", bytes.NewReader(good)); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Add on a checkout = %v, want ErrReadOnly", err)
	}
	if err := ro.Remove("body"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Remove on a checkout = %v, want ErrReadOnly", err)
	}
}
