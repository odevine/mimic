package template

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/manifest"
)

// goth reads a real font from the engine's built-in set, to stand in for a font a
// bundle ships
func goth(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../fonts/embedded/m15/Gotham-Medium.otf")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// loadWithFonts loads the small manifest after f has changed it, with files in the
// bundle
func loadWithFonts(t *testing.T, files map[string][]byte, f func(m *manifest.Manifest)) (*Template, error) {
	t.Helper()
	raw, err := os.ReadFile("../manifest/testdata/small.json")
	if err != nil {
		t.Fatal(err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Contract = manifest.Contract
	f(&m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, _, err := Load(&memAssets{manifest: out, files: files})
	return tmpl, err
}

func TestATemplateWithNoFontsHasTheStandardRoles(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	roles := tmpl.Fonts().Roles()
	if len(roles) != 6 || roles[0].Name != "title" || roles[0].Bundled {
		t.Errorf("roles = %+v, want the six standard ones on built-in fonts", roles)
	}
}

func TestLoadReadsTheFontsABundleShips(t *testing.T) {
	raw := goth(t)
	sum := sha256.Sum256(raw)
	tmpl, err := loadWithFonts(t, map[string][]byte{"font/Script.otf": raw}, func(m *manifest.Manifest) {
		m.Fonts = map[string]manifest.FontFile{"script": {File: "font/Script.otf", SHA256: hex.EncodeToString(sum[:])}}
		m.Roles = map[string]manifest.FontRole{
			"saga-chapter": {Label: "Chapter text", Font: "script"},
			"title":        {Builtin: "mimic-m15-info"},
		}
		box := m.Layouts["normal"].TextBoxes["title"]
		box.Text.Font = "saga-chapter"
		m.Layouts["normal"].TextBoxes["title"] = box
	})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	for _, r := range tmpl.Fonts().Roles() {
		byName[r.Name] = r.Family
	}
	if byName["saga-chapter"] != "Gotham" || byName["title"] != "Gotham" {
		t.Errorf("role fonts = %v, want the bundled font and the redefined title both on Gotham", byName)
	}
	if byName["body"] != "Plantin MT Pro" {
		t.Errorf("body = %q, want it left on its standard font", byName["body"])
	}
}

func TestLoadRefusesAFontItCannotUse(t *testing.T) {
	raw := goth(t)
	roles := func(m *manifest.Manifest) {
		m.Roles = map[string]manifest.FontRole{"r": {Font: "f"}}
	}
	cases := []struct {
		name  string
		files map[string][]byte
		edit  func(m *manifest.Manifest)
		want  string
	}{
		{"a font missing from the bundle", nil, func(m *manifest.Manifest) {
			m.Fonts = map[string]manifest.FontFile{"f": {File: "font/Gone.otf"}}
			roles(m)
		}, "font \"f\""},
		{"a file that is not a font", map[string][]byte{"font/F.otf": []byte("nope")}, func(m *manifest.Manifest) {
			m.Fonts = map[string]manifest.FontFile{"f": {File: "font/F.otf"}}
			roles(m)
		}, "is not a font"},
		{"a hash that does not match", map[string][]byte{"font/F.otf": raw}, func(m *manifest.Manifest) {
			m.Fonts = map[string]manifest.FontFile{"f": {File: "font/F.otf", SHA256: strings.Repeat("0", 64)}}
			roles(m)
		}, "does not match its sha256"},
	}
	for _, tc := range cases {
		tmpl, err := loadWithFonts(t, tc.files, tc.edit)
		if err == nil || tmpl != nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want a refusal mentioning %q", tc.name, err, tc.want)
		}
	}
}

func TestLoadRefusesATextBoxInAnUndefinedRole(t *testing.T) {
	_, err := loadWithFonts(t, nil, func(m *manifest.Manifest) {
		box := m.Layouts["normal"].TextBoxes["title"]
		box.Text.Font = "saga-chapter"
		m.Layouts["normal"].TextBoxes["title"] = box
	})
	if err == nil || !strings.Contains(err.Error(), "is not a font role") {
		t.Errorf("err = %v, want the undefined role refused", err)
	}
}

func TestALetterTheRolesFontLacksIsMeasuredAndDrawnFromTheNextFont(t *testing.T) {
	// The title font has no glyph for Ć, which it once drew as an empty space of
	// its own choosing. The body font has it, so the line takes that letter's room
	// and shows it
	src := fonts.Default(fonts.Title)
	box := TextBoxSpec{X: 10, Y: 110, Width: 600, Height: 120, FontSize: 60, VAlign: "baseline", Align: "left"}

	body, err := fonts.Default(fonts.Body).Face(box.FontSize)
	if err != nil {
		t.Fatal(err)
	}
	adv, _ := body.GlyphAdvance('Ć')
	span, err := MeasureTextSpan(box, TextPart{Text: "Ć", Src: src})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := span.Right-span.Left, adv.Round(); got < want-1 || got > want+1 {
		t.Errorf("Ć measures %d wide, want about the body font's %d", got, want)
	}

	inkRows := func(text string) int {
		res, err := RenderTextBox(box, 640, 240, TextPart{Text: text, Src: src})
		if err != nil {
			t.Fatal(err)
		}
		top, bottom := -1, -1
		if res.Image == nil {
			return 0
		}
		b := res.Image.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if res.Image.RGBAAt(x, y).A != 0 {
					if top < 0 {
						top = y
					}
					bottom = y
					break
				}
			}
		}
		if top < 0 {
			return 0
		}
		return bottom - top + 1
	}
	if plain, accented := inkRows("C"), inkRows("Ć"); accented <= plain {
		t.Errorf("Ć has %d rows of ink and C has %d, so the accent was not drawn", accented, plain)
	}
}
