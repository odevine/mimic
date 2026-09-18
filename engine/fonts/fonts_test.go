package fonts

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/basicfont"
)

func TestResolveEmbeddedRoles(t *testing.T) {
	// Roles with an embedded default resolve without hitting the fallback.
	for _, role := range []Role{Title, Body, BodyItalic, Mana, Symbols} {
		face, fallback, err := Resolve(role, "", 24)
		if err != nil {
			t.Errorf("Resolve(role %d): %v", role, err)
			continue
		}
		if fallback {
			t.Errorf("role %d used the basicfont fallback, want embedded default", role)
		}
		if face == nil {
			t.Errorf("role %d returned a nil face", role)
		}
	}
}

func TestResolveUnknownRoleFallsBack(t *testing.T) {
	// A role with no embedded default falls back to basicfont.
	face, fallback, err := Resolve(Role(-1), "", 24)
	if err != nil {
		t.Fatalf("Resolve(unknown role): %v", err)
	}
	if !fallback {
		t.Error("an unmapped role should report the basicfont fallback")
	}
	if face != basicfont.Face7x13 {
		t.Error("fallback should be basicfont.Face7x13")
	}
}

func TestResolvePrefersUserOverride(t *testing.T) {
	// A file whose name matches the Title role stems is used ahead of the
	// embedded default. Any real font file works as the stand-in Beleren.
	dir := t.TempDir()
	src, err := embedded.ReadFile("embedded/body/Merriweather-Regular.ttf")
	if err != nil {
		t.Fatalf("reading embedded font: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Beleren-Bold.ttf"), src, 0o644); err != nil {
		t.Fatalf("writing override: %v", err)
	}
	face, fallback, err := Resolve(Title, dir, 24)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if fallback || face == nil {
		t.Errorf("expected the override face, got fallback=%v face=%v", fallback, face)
	}
}

func TestFindUserFontItalicSplit(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"Plantin-Regular.ttf", "Plantin-Italic.ttf"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := filepath.Base(findUserFont(Body, dir)); got != "Plantin-Regular.ttf" {
		t.Errorf("Body matched %q, want the non-italic file", got)
	}
	if got := filepath.Base(findUserFont(BodyItalic, dir)); got != "Plantin-Italic.ttf" {
		t.Errorf("BodyItalic matched %q, want the italic file", got)
	}
}
