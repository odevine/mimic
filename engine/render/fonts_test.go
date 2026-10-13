package render

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

func fontBytes(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "fonts", "embedded", "m15", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// drawn renders the bolt at the smallest size and returns its pixels
func drawn(t *testing.T, tmpl *template.Template, edit func(req *template.RenderRequest)) []byte {
	t.Helper()
	req := renderBolt(t, nil)
	atMinDPI(tmpl, req)
	if edit != nil {
		edit(req)
	}
	buf, err := Render(context.Background(), tmpl, *req)
	if err != nil {
		t.Fatal(err)
	}
	return buf.ToImage(8).(*image.NRGBA).Pix
}

func TestRenderDrawsAnOverrideFromThePool(t *testing.T) {
	tmpl := placeholderTemplate(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "g.otf"), fontBytes(t, "Gotham-Medium.otf"), 0o644); err != nil {
		t.Fatal(err)
	}
	pool := fonts.NewPool(dir)

	plain := drawn(t, tmpl, nil)
	over := drawn(t, tmpl, func(r *template.RenderRequest) {
		r.FontPool, r.FontOverrides = pool, map[string]string{"title": "Gotham Medium", "body": "Gotham Medium"}
	})
	if bytes.Equal(plain, over) {
		t.Error("assigning a font to the title and body roles changed nothing")
	}
	// A name the pool lacks, or no pool at all, leaves each role on its own font.
	for name, edit := range map[string]func(*template.RenderRequest){
		"a missing font": func(r *template.RenderRequest) {
			r.FontPool, r.FontOverrides = pool, map[string]string{"title": "No Such Font Regular"}
		},
		"no pool": func(r *template.RenderRequest) { r.FontOverrides = map[string]string{"title": "Gotham Medium"} },
	} {
		if !bytes.Equal(plain, drawn(t, tmpl, edit)) {
			t.Errorf("%s changed the render", name)
		}
	}
}

func TestRenderDrawsATemplatesBundledFontInItsOwnRole(t *testing.T) {
	base := PlaceholderAssets()
	files := map[string][]byte{"font/Script.otf": fontBytes(t, "Gotham-Medium.otf")}
	load := func(role manifest.FontRole) *template.Template {
		m := readManifest(t, base)
		m.Contract = manifest.Contract
		m.Fonts = map[string]manifest.FontFile{"script": {File: "font/Script.otf"}}
		m.Roles = map[string]manifest.FontRole{"saga": role}
		l := m.Layouts["normal"]
		b := l.TextBoxes["title"]
		b.Text.Font = "saga"
		l.TextBoxes["title"] = b
		m.Layouts["normal"] = l
		return mustLoad(t, withManifest(t, base, m, files))
	}
	bundled := drawn(t, load(manifest.FontRole{Font: "script"}), nil)
	builtin := drawn(t, load(manifest.FontRole{Builtin: "mimic-m15-title"}), nil)
	if bytes.Equal(bundled, builtin) {
		t.Error("a role on a bundled font drew the same as one on a built-in font")
	}

	// The user can assign a pool font to a role the template made up.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.ttf"), fontBytes(t, "plantin-italic.ttf"), 0o644); err != nil {
		t.Fatal(err)
	}
	over := drawn(t, load(manifest.FontRole{Font: "script"}), func(r *template.RenderRequest) {
		r.FontPool, r.FontOverrides = fonts.NewPool(dir), map[string]string{"saga": "Plantin MT Pro Italic"}
	})
	if bytes.Equal(bundled, over) {
		t.Error("overriding the template's own role changed nothing")
	}
}
