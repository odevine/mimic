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
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "engine", "fonts", "embedded", "m15", name))
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

func TestAddStoresFlatAndReplacesByFileName(t *testing.T) {
	d := Dir{Path: t.TempDir(), Kind: KindManaged}
	first, err := d.Add("Mine.ttf", bytes.NewReader(realFont(t, "plantin-body.ttf")))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if first != filepath.Join(d.Path, "Mine.ttf") {
		t.Errorf("stored at %s, want directly in the folder", first)
	}
	// A path in the name is cut down to the file name, and the same name replaces.
	again, err := d.Add("../Mine.ttf", bytes.NewReader(realFont(t, "plantin-italic.ttf")))
	if err != nil {
		t.Fatalf("Add again: %v", err)
	}
	if again != first {
		t.Errorf("second add stored at %s, want %s", again, first)
	}
	other, err := d.Add("Other.otf", bytes.NewReader(realFont(t, "Gotham-Medium.otf")))
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(d.Path)
	if len(entries) != 2 {
		t.Errorf("folder holds %d entries, want the two fonts and no leftovers", len(entries))
	}
	got := NewPool(d).Get().Fonts()
	if len(got) != 2 {
		t.Fatalf("pool = %+v, want both fonts", got)
	}
	if f, ok := NewPool(d).Get().Lookup("Plantin MT Pro Italic"); !ok || f.Path != first {
		t.Errorf("the replaced file reads as %+v, want the italic", f)
	}

	if err := d.Remove(first); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("font still present after Remove: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("Remove took another font with it: %v", err)
	}
	if err := d.Remove(first); err != nil {
		t.Errorf("removing a font already gone: %v", err)
	}
}

func TestAddRejects(t *testing.T) {
	d := Dir{Path: t.TempDir(), Kind: KindManaged}
	good := realFont(t, "plantin-body.ttf")
	cases := []struct {
		name, file string
		raw        []byte
	}{
		{"not a font", "x.ttf", []byte("not a font")},
		{"wrong extension", "x.woff2", good},
		{"a hidden name", ".x.ttf", good},
	}
	for _, tc := range cases {
		if _, err := d.Add(tc.file, bytes.NewReader(tc.raw)); err == nil {
			t.Errorf("%s: Add accepted it", tc.name)
		}
	}
	if entries, _ := os.ReadDir(d.Path); len(entries) != 0 {
		t.Errorf("a rejected upload left %d files behind", len(entries))
	}

	ro := Dir{Path: t.TempDir(), Kind: KindCheckout}
	if _, err := ro.Add("x.ttf", bytes.NewReader(good)); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Add on a checkout = %v, want ErrReadOnly", err)
	}
	if err := ro.Remove(filepath.Join(ro.Path, "x.ttf")); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Remove on a checkout = %v, want ErrReadOnly", err)
	}
}

func TestRemoveStaysInsideTheFolder(t *testing.T) {
	outside := t.TempDir()
	victim := filepath.Join(outside, "Keep.ttf")
	if err := os.WriteFile(victim, realFont(t, "plantin-body.ttf"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := Dir{Path: filepath.Join(t.TempDir(), "fonts"), Kind: KindManaged}
	if err := os.MkdirAll(d.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	notFont := filepath.Join(d.Path, "notes.txt")
	if err := os.WriteFile(notFont, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{victim, filepath.Join(d.Path, "..", "..", filepath.Base(outside), "Keep.ttf"), d.Path, notFont} {
		if err := d.Remove(path); err == nil {
			t.Errorf("Remove(%s) was accepted", path)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("a font outside the folder was touched: %v", err)
	}
	if _, err := os.Stat(notFont); err != nil {
		t.Errorf("a file that is not a font was removed: %v", err)
	}
}

func TestPoolKeepsItsIndexUntilRefreshed(t *testing.T) {
	d := Dir{Path: t.TempDir(), Kind: KindManaged}
	p := NewPool(d)
	if len(p.Get().Fonts()) != 0 {
		t.Fatal("a new folder has fonts")
	}
	if _, err := d.Add("Mine.ttf", bytes.NewReader(realFont(t, "plantin-body.ttf"))); err != nil {
		t.Fatal(err)
	}
	if len(p.Get().Fonts()) != 0 {
		t.Error("the index changed without a refresh, so a render would walk the folder")
	}
	if got := p.Refresh().Fonts(); len(got) != 1 || len(p.Get().Fonts()) != 1 {
		t.Errorf("after Refresh the index holds %d fonts, want 1", len(got))
	}
	if len(NewPool(Dir{Kind: KindNone}).Get().Fonts()) != 0 {
		t.Error("a pool with no folder has fonts")
	}
}

func writeFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFlattenRoleFoldersMovesFontsUpAndKeepsTheOneARoleUsed(t *testing.T) {
	d := Dir{Path: t.TempDir(), Kind: KindManaged}
	body, italic, gotham := realFont(t, "plantin-body.ttf"), realFont(t, "plantin-italic.ttf"), realFont(t, "Gotham-Medium.otf")
	writeFile(t, filepath.Join(d.Path, "body", "a-first.ttf"), body)
	writeFile(t, filepath.Join(d.Path, "body", "b-second.ttf"), italic)
	writeFile(t, filepath.Join(d.Path, "type", "Caps.otf"), gotham)
	writeFile(t, filepath.Join(d.Path, "info", "notes.txt"), []byte("mine"))
	writeFile(t, filepath.Join(d.Path, "Existing.ttf"), body)
	// A name already taken at the top goes up under its folder's name.
	writeFile(t, filepath.Join(d.Path, "title", "Existing.ttf"), italic)

	moved := d.FlattenRoleFolders()
	got := map[string]Moved{}
	for _, m := range moved {
		got[filepath.Base(m.Path)] = m
	}
	if len(moved) != 4 {
		t.Fatalf("moved %+v, want 4 fonts", moved)
	}
	for name, want := range map[string]Moved{
		"a-first.ttf":        {Role: "body", Used: true},
		"b-second.ttf":       {Role: "body", Used: false},
		"Caps.otf":           {Role: "small-caps", Used: true},
		"title-Existing.ttf": {Role: "title", Used: true},
	} {
		m, ok := got[name]
		if !ok || m.Role != want.Role || m.Used != want.Used || m.Path != filepath.Join(d.Path, name) {
			t.Errorf("%s = %+v, want %+v at the top", name, m, want)
		}
		if _, err := os.Stat(filepath.Join(d.Path, name)); err != nil {
			t.Errorf("%s is not at the top: %v", name, err)
		}
	}
	if raw, _ := os.ReadFile(filepath.Join(d.Path, "Existing.ttf")); !bytes.Equal(raw, body) {
		t.Error("a font already at the top was replaced")
	}
	for _, gone := range []string{"body", "type", "title"} {
		if _, err := os.Stat(filepath.Join(d.Path, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the %s folder is still there after it was emptied", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(d.Path, "info", "notes.txt")); err != nil {
		t.Errorf("a folder holding something other than fonts lost it: %v", err)
	}
	if again := d.FlattenRoleFolders(); len(again) != 0 {
		t.Errorf("a second call moved %+v, want nothing left to do", again)
	}
}

func TestFlattenRoleFoldersLeavesAFolderTheAppDoesNotOwn(t *testing.T) {
	d := Dir{Path: t.TempDir(), Kind: KindCheckout}
	writeFile(t, filepath.Join(d.Path, "title", "Mine.ttf"), realFont(t, "plantin-body.ttf"))
	if moved := d.FlattenRoleFolders(); moved != nil {
		t.Errorf("moved %+v out of a folder that is read-only", moved)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "title", "Mine.ttf")); err != nil {
		t.Errorf("a font in a read-only folder moved: %v", err)
	}
}
