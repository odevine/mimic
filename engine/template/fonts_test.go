package template

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

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
