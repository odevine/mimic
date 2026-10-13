package fonts

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/basicfont"
)

func TestResolveEmbeddedRoles(t *testing.T) {
	// Every standard role resolves to its built-in font without hitting the
	// fallback.
	for _, info := range Roles() {
		face, fallback, err := Resolve(info.Role, "", 24)
		if err != nil {
			t.Errorf("Resolve(%s): %v", info.Role, err)
			continue
		}
		if fallback {
			t.Errorf("role %s used the basicfont fallback, want its built-in font", info.Role)
		}
		if face == nil {
			t.Errorf("role %s returned a nil face", info.Role)
		}
	}
}

func TestBuiltinNamesAreStable(t *testing.T) {
	// A template names a built-in by id, so each id must keep drawing the same
	// font. The family and style come from the font's own name table.
	want := map[string][2]string{
		"mimic-m15-title":       {"Beleren Proxy", "Bold"},
		"mimic-m15-body":        {"Plantin MT Pro", "Regular"},
		"mimic-m15-body-italic": {"Plantin MT Pro", "Italic"},
		"mimic-m15-small-caps":  {"Beleren Small Caps", "Bold"},
		"mimic-m15-info":        {"Gotham", "Medium"},
		"mimic-m15-mana":        {"Mana", "Regular"},
	}
	if len(Builtins()) != len(want) {
		t.Errorf("%d built-ins, want %d", len(Builtins()), len(want))
	}
	for _, b := range Builtins() {
		f, err := fontFromEmbedded(b.File)
		if err != nil {
			t.Errorf("%s: %v", b.ID, err)
			continue
		}
		if family, style := names(f); [2]string{family, style} != want[b.ID] {
			t.Errorf("%s = %q %q, want %q", b.ID, family, style, want[b.ID])
		}
	}
}

func TestBuiltinsAreComplete(t *testing.T) {
	// Each built-in id is unique, its file is embedded and parses, and every
	// standard role names a built-in that exists.
	seen := map[string]bool{}
	for _, b := range Builtins() {
		if seen[b.ID] {
			t.Errorf("built-in id %q is listed twice", b.ID)
		}
		seen[b.ID] = true
		f, err := fontFromEmbedded(b.File)
		if err != nil {
			t.Errorf("built-in %q (%s): %v", b.ID, b.File, err)
			continue
		}
		if family, style := names(f); family == "" || style == "" {
			t.Errorf("built-in %q has no family or style in its name table", b.ID)
		}
	}
	for _, info := range Roles() {
		if !seen[info.Builtin] {
			t.Errorf("role %s names built-in %q, which does not exist", info.Role, info.Builtin)
		}
	}
}

func TestResolveUnknownRoleFallsBack(t *testing.T) {
	// A role the engine does not know falls back to basicfont.
	face, fallback, err := Resolve(Role("no-such-role"), "", 24)
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
	// A font dropped into the Title role folder is used ahead of the built-in
	// font, whatever its filename. Any real font file stands in for Beleren.
	dir := t.TempDir()
	src, err := embedded.ReadFile("embedded/m15/plantin-body.ttf")
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
		t.Fatal("Body resolved to the basicfont fallback, want the built-in font")
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
	s := ResolveFont(Role("no-such-role"), "")
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
		t.Fatal("Mana resolved to the basicfont fallback, want the built-in font")
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

func TestParseRole(t *testing.T) {
	cases := map[string]Role{
		"title":       Title,
		"body":        Body,
		"body-italic": BodyItalic,
		"mana":        Mana,
		"small-caps":  SmallCaps,
		"info":        Info,
	}
	for name, want := range cases {
		got, ok := ParseRole(name)
		if !ok {
			t.Errorf("ParseRole(%q) reported false, want %v", name, want)
			continue
		}
		if got != want {
			t.Errorf("ParseRole(%q) = %v, want %v", name, got, want)
		}
	}
	for _, name := range []string{"", "bold", "TITLE"} {
		if _, ok := ParseRole(name); ok {
			t.Errorf("ParseRole(%q) reported true, want false", name)
		}
	}
}
