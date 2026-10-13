package author

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

func gothamBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../fonts/embedded/m15/Gotham-Medium.otf")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// withFonts is the small template with a font, a role that uses it, and a title
// box set in that role
func withFonts(t *testing.T, fonts, roles string) fstest.MapFS {
	t.Helper()
	fsys := smallFS(t)
	tpl := strings.Replace(stdTemplate, `"contract": "1.0"`, `"contract": "1.1", "minEngine": "0.1.0"`, 1)
	tpl = strings.Replace(tpl, `"parts": {`, `"fonts": {`+fonts+`}, "roles": {`+roles+`}, "parts": {`, 1)
	fsys["template.json"] = &fstest.MapFile{Data: []byte(tpl)}
	layout := strings.Replace(stdLayout, `"fontSize": 12`, `"fontSize": 12, "font": "saga"`, 1)
	fsys["layouts/normal.json"] = &fstest.MapFile{Data: []byte(layout)}
	return fsys
}

const sagaRole = `"saga": {"label": "Saga text", "font": "script"}`

func TestBuildShipsTheFontsATemplateNames(t *testing.T) {
	fsys := withFonts(t, `"script": {"file": "font/Script.otf"}`, sagaRole)
	raw := gothamBytes(t)
	fsys["font/Script.otf"] = &fstest.MapFile{Data: raw}

	b := build(t, fsys)
	f := b.Manifest.Fonts["script"]
	if f.Family != "Gotham" || f.Style != "Medium" || len(f.SHA256) != 64 {
		t.Errorf("font = %+v, want the names and hash read from the file", f)
	}
	if !bytes.Equal(b.Assets["font/Script.otf"], raw) {
		t.Error("the bundle does not hold the font file at its path")
	}
	if b.Manifest.Roles["saga"].Font != "script" {
		t.Errorf("roles = %+v", b.Manifest.Roles)
	}

	// The bundle loads, and its role draws with the shipped font.
	var zipped bytes.Buffer
	if err := b.WriteZip(&zipped); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "t.mimic")
	if err := os.WriteFile(path, zipped.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := template.NewZipAssetProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	tmpl, ds, err := template.Load(p)
	if err != nil {
		t.Fatalf("Load: %v\n%v", err, ds)
	}
	var got roleSeen
	for _, r := range tmpl.Fonts().Roles() {
		if r.Name == "saga" {
			got = roleSeen{r.Family, r.Label, r.Bundled}
		}
	}
	if got != (roleSeen{"Gotham", "Saga text", true}) {
		t.Errorf("saga = %+v", got)
	}
}

type roleSeen struct {
	family, label string
	bundled       bool
}

func TestBuildKeepsADeclaredNameAndChecksADeclaredHash(t *testing.T) {
	fsys := withFonts(t, `"script": {"file": "font/Script.otf", "family": "House Script"}`, sagaRole)
	fsys["font/Script.otf"] = &fstest.MapFile{Data: gothamBytes(t)}
	if f := build(t, fsys).Manifest.Fonts["script"]; f.Family != "House Script" || f.Style != "Medium" {
		t.Errorf("font = %+v, want the declared family and the file's style", f)
	}

	bad := withFonts(t, `"script": {"file": "font/Script.otf", "sha256": "`+strings.Repeat("0", 64)+`"}`, sagaRole)
	bad["font/Script.otf"] = &fstest.MapFile{Data: gothamBytes(t)}
	wantError(t, buildErrors(t, bad), "fonts.script.file", "does not match the sha256")
}

func TestBuildReportsAFontItCannotShip(t *testing.T) {
	missing := withFonts(t, `"script": {"file": "font/Gone.otf"}`, sagaRole)
	wantError(t, buildErrors(t, missing), "fonts.script.file", "cannot be read")

	junk := withFonts(t, `"script": {"file": "font/Script.otf"}`, sagaRole)
	junk["font/Script.otf"] = &fstest.MapFile{Data: []byte("not a font")}
	wantError(t, buildErrors(t, junk), "fonts.script.file", "is not a font")

	elsewhere := withFonts(t, `"script": {"file": "assets/Script.otf"}`, sagaRole)
	elsewhere["assets/Script.otf"] = &fstest.MapFile{Data: gothamBytes(t)}
	wantError(t, buildErrors(t, elsewhere), "fonts.script.file", "directly in font/")

	old := withFonts(t, `"script": {"file": "font/Script.otf"}`, sagaRole)
	old["font/Script.otf"] = &fstest.MapFile{Data: gothamBytes(t)}
	tpl := strings.Replace(string(old["template.json"].Data), `"contract": "1.1"`, `"contract": "1.0"`, 1)
	old["template.json"] = &fstest.MapFile{Data: []byte(tpl)}
	wantError(t, buildErrors(t, old), "contract", "need contract 1.1")
}

func TestBuildWarnsWhenFontsComeWithNoMinimumEngine(t *testing.T) {
	fsys := withFonts(t, `"script": {"file": "font/Script.otf"}`, sagaRole)
	fsys["font/Script.otf"] = &fstest.MapFile{Data: gothamBytes(t)}
	tpl := strings.Replace(string(fsys["template.json"].Data), `, "minEngine": "0.1.0"`, "", 1)
	fsys["template.json"] = &fstest.MapFile{Data: []byte(tpl)}

	b, ds := Build(fsys, Options{})
	if b == nil || ds.HasErrors() {
		t.Fatalf("build failed: %v", ds)
	}
	for _, d := range ds {
		if d.Severity == manifest.Warning && strings.Contains(d.Path, "minEngine") {
			return
		}
	}
	t.Errorf("no minEngine warning in %v", ds)
}
