package fonts

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/basicfont"
)

func TestResolveEmbeddedRoles(t *testing.T) {
	// Roles with an embedded default resolve without hitting the fallback.
	for _, role := range []Role{Title, Body, BodyItalic, Mana} {
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

func TestResolveBorrowsFallbackRole(t *testing.T) {
	// SmallCaps and Info bundle no default of their own, so with no override
	// they borrow a fallback role's default rather than the bitmap face.
	for _, role := range []Role{SmallCaps, Info} {
		if _, ok := embeddedPath[role]; ok {
			t.Errorf("role %d unexpectedly has its own embedded default", role)
		}
		_, fallback, err := Resolve(role, "", 24)
		if err != nil {
			t.Errorf("Resolve(role %d): %v", role, err)
			continue
		}
		if fallback {
			t.Errorf("role %d used the basicfont fallback, want a borrowed default", role)
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
	// A font dropped into the Title role folder is used ahead of the embedded
	// default, whatever its filename. Any real font file stands in for Beleren.
	dir := t.TempDir()
	src, err := embedded.ReadFile("embedded/body/Merriweather-Regular.ttf")
	if err != nil {
		t.Fatalf("reading embedded font: %v", err)
	}
	titleDir := filepath.Join(dir, roleDir[Title])
	if err := os.MkdirAll(titleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titleDir, "AnyName.ttf"), src, 0o644); err != nil {
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

func TestFindUserFontUsesRoleFolder(t *testing.T) {
	dir := t.TempDir()
	bodyDir := filepath.Join(dir, roleDir[Body])
	if err := os.MkdirAll(bodyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bodyDir, "whatever.otf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A font in the body folder fills the Body role by folder, not by name.
	if got := filepath.Base(findUserFont(Body, dir)); got != "whatever.otf" {
		t.Errorf("Body matched %q, want whatever.otf", got)
	}
	// An empty role folder yields no override, so the caller uses the default.
	if got := findUserFont(BodyItalic, dir); got != "" {
		t.Errorf("BodyItalic matched %q, want no override", got)
	}
	// A file at the override root, in no role folder, is ignored.
	if err := os.WriteFile(filepath.Join(dir, "loose.ttf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findUserFont(Title, dir); got != "" {
		t.Errorf("Title matched %q from the root, want no override", got)
	}
}

func TestSizerBuildsFacesPerSize(t *testing.T) {
	// One resolved font produces distinct faces at different sizes, and a
	// bigger size has a taller line height.
	s := ResolveFont(Body, "")
	if s.Fallback() {
		t.Fatal("Body resolved to the basicfont fallback, want the embedded font")
	}
	small, err := s.Face(12)
	if err != nil {
		t.Fatalf("Face(12): %v", err)
	}
	large, err := s.Face(48)
	if err != nil {
		t.Fatalf("Face(48): %v", err)
	}
	if small.Metrics().Height >= large.Metrics().Height {
		t.Errorf("12pt height %v is not smaller than 48pt height %v",
			small.Metrics().Height, large.Metrics().Height)
	}
}

func TestSizerFallbackFace(t *testing.T) {
	s := ResolveFont(Role(-1), "")
	if !s.Fallback() {
		t.Error("an unmapped role should report the basicfont fallback")
	}
	face, err := s.Face(24)
	if err != nil {
		t.Fatalf("Face: %v", err)
	}
	if face != basicfont.Face7x13 {
		t.Error("fallback Face should be basicfont.Face7x13")
	}
}

func TestManaFaceHasPrivateUseGlyphs(t *testing.T) {
	// The Mana font carries its symbols in the private use area, not at ASCII
	// letters, so a face that resolves is not by itself proof it is usable.
	// The codepoints below are the ones the Mana cheatsheet documents
	face, fallback, err := Resolve(Mana, "", 32)
	if err != nil {
		t.Fatalf("Resolve(Mana): %v", err)
	}
	if fallback {
		t.Fatal("Mana resolved to the basicfont fallback, want the embedded font")
	}
	for _, tc := range []struct {
		r    rune
		name string
	}{
		{0xE600, "white mana"},
		{0xE601, "blue mana"},
		{0xE602, "black mana"},
		{0xE603, "red mana"},
		{0xE604, "green mana"},
		{0xE61A, "tap"},
		{0xE61B, "untap"},
		{0xE904, "colorless"},
	} {
		bounds, _, ok := face.GlyphBounds(tc.r)
		if !ok {
			t.Errorf("%s (U+%04X) is not mapped", tc.name, tc.r)
			continue
		}
		if bounds.Max.X <= bounds.Min.X || bounds.Max.Y <= bounds.Min.Y {
			t.Errorf("%s (U+%04X) maps to an empty glyph", tc.name, tc.r)
		}
	}
}
