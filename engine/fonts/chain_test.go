package fonts

import (
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// A letter the Beleren fonts lack and Plantin and Gotham have
const missing = 'Ć'

func face(t *testing.T, s *Sizer, size float64) font.Face {
	t.Helper()
	f, err := s.Face(size)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// plain is one font alone at a size, with no chain
func plain(t *testing.T, builtin string, size float64) font.Face {
	t.Helper()
	f, err := fontFromEmbedded(builtinFile[builtin])
	if err != nil {
		t.Fatal(err)
	}
	out, err := newFace(f, size)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestARuneTheTitleFontLacksIsDrawnFromTheBodyFont(t *testing.T) {
	title := Default(Title)
	f := face(t, title, 24)
	own := plain(t, "mimic-m15-title", 24)
	if _, ok := own.GlyphAdvance(missing); ok {
		t.Fatalf("%q is in the title font, so this test proves nothing", missing)
	}

	adv, ok := f.GlyphAdvance(missing)
	body := plain(t, "mimic-m15-body", 24)
	wantAdv, _ := body.GlyphAdvance(missing)
	if !ok || adv != wantAdv {
		t.Errorf("advance of %q = %v, %v, want the body font's %v", missing, adv, ok, wantAdv)
	}
	bounds, _, ok := f.GlyphBounds(missing)
	wantBounds, _, _ := body.GlyphBounds(missing)
	if !ok || bounds != wantBounds {
		t.Errorf("bounds of %q = %v, %v, want the body font's %v", missing, bounds, ok, wantBounds)
	}
	dr, mask, _, gadv, ok := f.Glyph(fixed.P(10, 40), missing)
	if !ok || mask == nil || dr.Empty() || gadv != wantAdv {
		t.Errorf("glyph of %q = %v, %v, %v, want something drawn with the body font's advance", missing, dr, ok, gadv)
	}
}

func TestARuneTheFirstFontHasIsDrawnFromIt(t *testing.T) {
	f := face(t, Default(Title), 24)
	own := plain(t, "mimic-m15-title", 24)
	for _, r := range "Aa Lightning Bolt 0" {
		got, ok := f.GlyphAdvance(r)
		want, _ := own.GlyphAdvance(r)
		if !ok || got != want {
			t.Errorf("advance of %q = %v, %v, want the title font's %v", r, got, ok, want)
		}
	}
	if f.Metrics() != own.Metrics() {
		t.Errorf("metrics = %+v, want the title font's %+v, so a line is as tall as it was", f.Metrics(), own.Metrics())
	}
	if got, want := f.Kern('A', 'V'), own.Kern('A', 'V'); got != want {
		t.Errorf("kern A V = %v, want the title font's %v", got, want)
	}
}

func TestARuneNoFontHasIsStillMissing(t *testing.T) {
	f := face(t, Default(Title), 24)
	if _, ok := f.GlyphAdvance('一'); ok {
		t.Error("a CJK ideograph was reported present")
	}
	if _, _, _, _, ok := f.Glyph(fixed.P(0, 0), '一'); ok {
		t.Error("a CJK ideograph was drawn")
	}
}

func TestAPairSplitAcrossFontsIsNotKerned(t *testing.T) {
	f := face(t, Default(Title), 24)
	if k := f.Kern('A', missing); k != 0 {
		t.Errorf("kern between a title glyph and a body glyph = %v, want 0", k)
	}
}

func TestARoleWithNoGapsKeepsAPlainFace(t *testing.T) {
	if _, chained := face(t, Default(Body), 24).(*chainFace); chained {
		t.Error("the body role has the body font and nothing to fall back to, but got a chain")
	}
	if _, chained := face(t, Default(Title), 24).(*chainFace); !chained {
		t.Error("the title role got no fallback, so its missing letters would be dropped")
	}
}

func TestAnAssignedFontFallsBackToTheRolesOwnFontBeforeTheBodyFont(t *testing.T) {
	dir := t.TempDir()
	// The small caps font lacks the letter, as the title font does. The role's own
	// font, Gotham, has it, and so does the body font, with a different width
	writeFont(t, dir, "caps.ttf", "embedded/m15/Beleren Small Caps.ttf")
	r := NewResolver(nil, NewPool(dir), map[string]string{"info": "Beleren Small Caps Bold"})
	f := face(t, r.Font(Info), 24)

	got, ok := f.GlyphAdvance(missing)
	gotham := plain(t, "mimic-m15-info", 24)
	body := plain(t, "mimic-m15-body", 24)
	wantGotham, _ := gotham.GlyphAdvance(missing)
	wantBody, _ := body.GlyphAdvance(missing)
	if wantGotham == wantBody {
		t.Fatal("the two fonts give the letter the same width, so the test cannot tell them apart")
	}
	if !ok || got != wantGotham {
		t.Errorf("advance = %v, %v, want Gotham's %v and not the body font's %v", got, ok, wantGotham, wantBody)
	}
	// What the assigned font has, it draws itself.
	caps := plain(t, "mimic-m15-small-caps", 24)
	if a, _ := f.GlyphAdvance('A'); a != mustAdvance(t, caps, 'A') {
		t.Errorf("advance of A = %v, want the assigned font's", a)
	}
}

func mustAdvance(t *testing.T, f font.Face, r rune) fixed.Int26_6 {
	t.Helper()
	a, ok := f.GlyphAdvance(r)
	if !ok {
		t.Fatalf("no glyph for %q", r)
	}
	return a
}

func TestBundledFontsFallBackToTheBodyFont(t *testing.T) {
	raw := embeddedBytes(t, "embedded/m15/BelerenProxy-Bold.ttf")
	cat, err := NewCatalog(
		map[string]FontDef{"f": {File: "font/F.ttf"}},
		map[string]RoleDef{"saga": {Font: "f"}},
		memOpen(map[string][]byte{"font/F.ttf": raw}),
	)
	if err != nil {
		t.Fatal(err)
	}
	f := face(t, NewResolver(cat, nil, nil).Font("saga"), 24)
	if _, ok := f.GlyphAdvance(missing); !ok {
		t.Errorf("a role on a bundled font dropped %q", missing)
	}
}

func TestAChainFaceSizesItsFontsTogether(t *testing.T) {
	small, big := face(t, Default(Title), 12), face(t, Default(Title), 48)
	a, _ := small.GlyphAdvance(missing)
	b, _ := big.GlyphAdvance(missing)
	if b < 3*a {
		t.Errorf("advance at 48 is %v and at 12 is %v, want a fallback glyph that scales with the size", b, a)
	}
}

func TestAChainFaceCloses(t *testing.T) {
	f := face(t, Default(Title), 24)
	f.GlyphAdvance(missing)
	if err := f.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
