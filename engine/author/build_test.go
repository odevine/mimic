package author

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

// pngOf is a size by size image with fill painted into rect
func pngOf(t *testing.T, size int, rect image.Rectangle, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const stdLayout = `{
  "serves": [{"role": "single", "kind": "standard"}],
  "canvas": {"width": 100, "height": 100},
  "stack": [{"part": "bg"}, {"id": "mark", "part": "mark", "at": {"x": 100, "y": 50}}],
  "textBoxes": {"title": {"rect": {"x": 0, "y": 0, "width": 50, "height": 10}, "text": {"fontSize": 12}}}
}`

const stdTemplate = `{
  "contract": "1.0", "id": "test.small", "version": "1.2.3", "name": "Small",
  "parts": {
    "bg":   {"slot": "background", "origin": {"x": 4, "y": 6}},
    "mark": {"variants": {"any": "marks/a.png"}}
  }
}`

// smallFS is a template with a part cut from a larger sheet
func smallFS(t *testing.T) fstest.MapFS {
	t.Helper()
	red, blue := color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 0, 255, 255}
	return fstest.MapFS{
		"template.json":       {Data: []byte(stdTemplate)},
		"layouts/normal.json": {Data: []byte(stdLayout)},
		"bg/w.png":            {Data: pngOf(t, 40, image.Rect(10, 10, 20, 30), red)},
		"bg/u.png":            {Data: pngOf(t, 40, image.Rect(15, 12, 30, 25), blue)},
		"marks/a.png":         {Data: pngOf(t, 40, image.Rect(0, 0, 40, 40), red)},
	}
}

func build(t *testing.T, fsys fstest.MapFS) *Built {
	t.Helper()
	b, ds := Build(fsys, Options{})
	if b == nil {
		t.Fatalf("build failed: %v", ds)
	}
	return b
}

func buildErrors(t *testing.T, fsys fstest.MapFS) manifest.Diagnostics {
	t.Helper()
	b, ds := Build(fsys, Options{})
	if b != nil || !ds.HasErrors() {
		t.Fatalf("build succeeded, want errors: %v", ds)
	}
	return ds
}

func wantError(t *testing.T, ds manifest.Diagnostics, path, contains string) {
	t.Helper()
	for _, d := range ds {
		if d.Severity == manifest.Error && strings.Contains(d.Path, path) && strings.Contains(d.Message, contains) {
			return
		}
	}
	t.Errorf("no error at %q containing %q in %v", path, contains, ds)
}

func TestBuildCutsAPartToTheUnionOfItsImages(t *testing.T) {
	b := build(t, smallFS(t))
	p := b.Manifest.Parts["bg"]
	// The images' visible pixels span x 10 to 30 and y 10 to 30, and the part's own
	// origin is added to where the cut sits
	if p.Origin != (manifest.Point{X: 14, Y: 16}) {
		t.Errorf("origin = %v, want 14,16", p.Origin)
	}
	for key, v := range p.Variants {
		img, err := png.Decode(bytes.NewReader(b.Assets[v.Path]))
		if err != nil {
			t.Fatal(err)
		}
		if got := img.Bounds().Size(); got != (image.Point{X: 20, Y: 20}) {
			t.Errorf("%s is %v, want 20x20", key, got)
		}
	}
	// The u image sat at 15,12, so it is at 5,2 inside the cut
	img, _ := png.Decode(bytes.NewReader(b.Assets[p.Variants["u"].Path]))
	if _, _, bl, a := img.At(5, 2).RGBA(); bl == 0 || a == 0 {
		t.Error("the u image moved inside the cut")
	}
}

func TestBuildLeavesAPartThatFillsItsSheetAlone(t *testing.T) {
	fsys := smallFS(t)
	b := build(t, fsys)
	v := b.Manifest.Parts["mark"].Variants["any"]
	if !bytes.Equal(b.Assets[v.Path], fsys["marks/a.png"].Data) {
		t.Error("an image that needed no cut was encoded again")
	}
	if b.Manifest.Parts["mark"].Origin != (manifest.Point{}) {
		t.Errorf("origin = %v", b.Manifest.Parts["mark"].Origin)
	}
}

func TestBuildMovesAnEntryThatPlacesACutPart(t *testing.T) {
	fsys := smallFS(t)
	fsys["layouts/normal.json"] = &fstest.MapFile{Data: []byte(strings.Replace(stdLayout, `"part": "mark", "at"`, `"part": "bg", "at"`, 1))}
	b := build(t, fsys)
	var at *manifest.Point
	for _, e := range b.Manifest.Layouts["normal"].Stack {
		if e.ID == "mark" {
			at = e.At
		}
	}
	// The entry put the sheet at 100,50, and the cut part starts 10,10 into it
	if at == nil || *at != (manifest.Point{X: 110, Y: 60}) {
		t.Errorf("at = %v, want 110,60", at)
	}
}

func TestBuildNamesImagesByContent(t *testing.T) {
	fsys := smallFS(t)
	fsys["marks/b.png"] = fsys["marks/a.png"]
	p := `"mark": {"slot": "pinlines", "variants": {"w": "marks/a.png", "u": "marks/b.png"}}`
	fsys["template.json"] = &fstest.MapFile{Data: []byte(strings.Replace(stdTemplate, `"mark": {"variants": {"any": "marks/a.png"}}`, p, 1))}
	b := build(t, fsys)
	vs := b.Manifest.Parts["mark"].Variants
	if vs["w"].Path != vs["u"].Path {
		t.Errorf("identical images have two names: %v", vs)
	}
	if len(b.Assets) != 3 {
		t.Errorf("%d assets, want the two of bg and the one shared", len(b.Assets))
	}
	if !strings.HasPrefix(vs["w"].Path, "assets/") || !strings.HasSuffix(vs["w"].Path, ".png") {
		t.Errorf("path = %q", vs["any"].Path)
	}
}

func TestBuildWritesTheSameBundleEveryTimeAndTheEngineLoadsIt(t *testing.T) {
	var a, b bytes.Buffer
	if err := build(t, smallFS(t)).WriteZip(&a); err != nil {
		t.Fatal(err)
	}
	if err := build(t, smallFS(t)).WriteZip(&b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("two builds of one folder differ")
	}
	path := filepath.Join(t.TempDir(), "small.mimic")
	if err := os.WriteFile(path, a.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := template.NewZipAssetProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	tmpl, ds, err := template.Load(p)
	if err != nil {
		t.Fatalf("load: %v %v", err, ds)
	}
	if tmpl.ID() != "test.small" {
		t.Errorf("id = %q", tmpl.ID())
	}
}

func TestBuildReportsEveryProblemInOnePass(t *testing.T) {
	fsys := smallFS(t)
	fsys["bg/r.png"] = &fstest.MapFile{Data: pngOf(t, 30, image.Rect(1, 1, 5, 5), color.NRGBA{A: 255})}
	fsys["template.json"] = &fstest.MapFile{Data: []byte(strings.Replace(strings.Replace(stdTemplate, `"1.2.3"`, `"one"`, 1),
		`"mark": {"variants": {"any": "marks/a.png"}}`, `"mark": {"dir": "nowhere"}, "empty": {"variants": {"any": "marks/blank.png"}}`, 1))}
	fsys["marks/blank.png"] = &fstest.MapFile{Data: pngOf(t, 8, image.Rectangle{}, color.NRGBA{})}
	ds := buildErrors(t, fsys)
	wantError(t, ds, "version", "not a version")
	wantError(t, ds, "parts.mark.dir", "cannot be read")

	// Past the first group of problems, the images are read and checked
	fsys["template.json"] = &fstest.MapFile{Data: []byte(strings.Replace(stdTemplate, `"mark": {"variants": {"any": "marks/a.png"}}`, `"empty": {"variants": {"any": "marks/blank.png"}}, "mark": {"variants": {"any": "marks/a.png"}}`, 1))}
	ds = buildErrors(t, fsys)
	wantError(t, ds, "parts.bg", "one size")
	wantError(t, ds, "parts.empty", "visible pixel")
}

func TestBuildRefusesWhatTheSourceDoesNotSay(t *testing.T) {
	fsys := smallFS(t)
	fsys["template.json"] = &fstest.MapFile{Data: []byte(strings.Replace(stdTemplate, `"name": "Small"`, `"name": "Small", "extends": "mimic.m15"`, 1))}
	wantError(t, buildErrors(t, fsys), "template.json", "unknown field")

	fsys = smallFS(t)
	delete(fsys, "layouts/normal.json")
	wantError(t, buildErrors(t, fsys), "layouts", "")

	fsys = smallFS(t)
	fsys["layouts/normal.json"] = &fstest.MapFile{Data: []byte(strings.Replace(stdLayout, `"part": "bg"}`, `"part": "missing"}`, 1))}
	wantError(t, buildErrors(t, fsys), "stack[0].part", "")
}

func TestBuildTakesAVersionFromTheCaller(t *testing.T) {
	b, ds := Build(smallFS(t), Options{Version: "2.0.0-rc.1"})
	if b == nil {
		t.Fatal(ds)
	}
	if b.Header.Version != "2.0.0-rc.1" || b.Header.Format != template.BundleFormat || b.Header.ID != "test.small" {
		t.Errorf("header = %+v", b.Header)
	}
}

// layoutsOf resolves a set of layout files written as JSON
func layoutsOf(t *testing.T, files map[string]string) (map[string]rawLayout, manifest.Diagnostics) {
	t.Helper()
	raw := map[string]rawLayout{}
	for name, src := range files {
		var l rawLayout
		if err := json.Unmarshal([]byte(src), &l); err != nil {
			t.Fatal(err)
		}
		raw[name] = l
	}
	return resolveLayouts(raw)
}

func keysOf(t *testing.T, l rawLayout) []string {
	t.Helper()
	var stack []json.RawMessage
	if err := json.Unmarshal(l["stack"], &stack); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range stack {
		k, _ := entryKey(e)
		out = append(out, k)
	}
	return out
}

const baseLayout = `{
  "serves": [{"role": "single", "kind": "standard"}],
  "canvas": {"width": 100, "height": 100},
  "stack": [{"part": "bg"}, {"part": "border"}, {"id": "pt", "part": "pt_box"}],
  "textBoxes": {"title": {"rect": {"x": 1}}, "rules": {"rect": {"x": 2}}},
  "art": [{"x": 1, "y": 1, "width": 5, "height": 5, "after": "bg"}]
}`

func TestExtendsStartsFromTheBaseAndAppliesThePatch(t *testing.T) {
	got, ds := layoutsOf(t, map[string]string{
		"normal": baseLayout,
		"walker": `{
		  "extends": ["normal"],
		  "serves": [{"role": "single", "kind": "planeswalker"}],
		  "patch": {
		    "stack": {
		      "remove": ["pt"],
		      "replace": [{"id": "border", "with": [{"part": "pw_border"}]}],
		      "insert": [{"after": "bg", "entries": [{"part": "band"}]}, {"before": "bg", "entries": [{"part": "under"}]}]
		    },
		    "textBoxes": {"rules": null, "title": {"rect": {"y": 9}}},
		    "canvas": {"width": 200}
		  }
		}`,
	})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	w := got["walker"]
	if keys := keysOf(t, w); strings.Join(keys, ",") != "under,bg,band,pw_border" {
		t.Errorf("stack = %v", keys)
	}
	var boxes map[string]json.RawMessage
	_ = json.Unmarshal(w["textBoxes"], &boxes)
	if _, ok := boxes["rules"]; ok || string(boxes["title"]) != `{"rect":{"x":1,"y":9}}` {
		t.Errorf("textBoxes = %s", w["textBoxes"])
	}
	if string(w["canvas"]) != `{"height":100,"width":200}` {
		t.Errorf("canvas = %s", w["canvas"])
	}
	if !strings.Contains(string(w["serves"]), "planeswalker") {
		t.Errorf("serves = %s", w["serves"])
	}
	if !strings.Contains(string(w["art"]), `"after":"bg"`) && !strings.Contains(string(w["art"]), `"after": "bg"`) {
		t.Errorf("art not inherited: %s", w["art"])
	}
	if keys := keysOf(t, got["normal"]); strings.Join(keys, ",") != "bg,border,pt" {
		t.Errorf("the base changed: %v", keys)
	}
}

func TestExtendsAStackOfItsOwnReplacesTheInheritedOne(t *testing.T) {
	got, ds := layoutsOf(t, map[string]string{
		"normal": baseLayout,
		"other":  `{"extends": ["normal"], "serves": [], "stack": [{"part": "solo"}]}`,
	})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	if keys := keysOf(t, got["other"]); strings.Join(keys, ",") != "solo" {
		t.Errorf("stack = %v", keys)
	}
	var boxes map[string]json.RawMessage
	_ = json.Unmarshal(got["other"]["textBoxes"], &boxes)
	if len(boxes) != 2 {
		t.Errorf("text boxes were not inherited: %s", got["other"]["textBoxes"])
	}
}

func TestExtendsAListCombinesItsBasesInOrder(t *testing.T) {
	got, ds := layoutsOf(t, map[string]string{
		"a": baseLayout,
		"b": `{"serves": [], "stack": [{"part": "border"}, {"part": "extra"}], "textBoxes": {"rules": {"rect": {"x": 7}}, "name": {"rect": {"x": 3}}}}`,
		"c": `{"extends": ["a", "b"], "serves": []}`,
	})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	c := got["c"]
	if keys := keysOf(t, c); strings.Join(keys, ",") != "bg,border,pt,extra" {
		t.Errorf("stack = %v", keys)
	}
	var boxes map[string]json.RawMessage
	_ = json.Unmarshal(c["textBoxes"], &boxes)
	if len(boxes) != 3 || string(boxes["rules"]) != `{"rect": {"x": 7}}` && string(boxes["rules"]) != `{"rect":{"x":7}}` {
		t.Errorf("textBoxes = %s", c["textBoxes"])
	}
}

func TestExtendsRefusesWhatItCannotResolve(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		path  string
		text  string
	}{
		{"missing base", map[string]string{"x": `{"extends": ["nope"], "serves": []}`}, "layouts.x.extends[0]", "not a layout"},
		{"cycle", map[string]string{"x": `{"extends": ["y"], "serves": []}`, "y": `{"extends": ["x"], "serves": []}`}, "extends", "itself"},
		{"unknown stack entry", map[string]string{"n": baseLayout, "x": `{"extends": ["n"], "serves": [], "patch": {"stack": {"remove": ["ghost"]}}}`}, "patch.stack.remove[0]", "no entry"},
		{"insert without a place", map[string]string{"n": baseLayout, "x": `{"extends": ["n"], "serves": [], "patch": {"stack": {"insert": [{"entries": []}]}}}`}, "insert[0]", "after and before"},
		{"patch without extends", map[string]string{"x": strings.Replace(baseLayout, `"stack"`, `"patch": {}, "stack"`, 1)}, "layouts.x.patch", "extends none"},
		{"field beside extends", map[string]string{"n": baseLayout, "x": `{"extends": ["n"], "serves": [], "textBoxes": {}}`}, "layouts.x.textBoxes", "patch"},
		{"unknown field", map[string]string{"x": strings.Replace(baseLayout, `"canvas"`, `"bogus": 1, "canvas"`, 1)}, "layouts.x.bogus", "not a field"},
		{"no serves", map[string]string{"x": `{"stack": [], "textBoxes": {}}`}, "layouts.x.serves", "which shapes"},
		{"unknown patch field", map[string]string{"n": baseLayout, "x": `{"extends": ["n"], "serves": [], "patch": {"nope": 1}}`}, "layouts.x.patch", "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds := layoutsOf(t, tt.files)
			wantError(t, ds, tt.path, tt.text)
		})
	}
}

func TestExtendsInheritsAndPatchesSections(t *testing.T) {
	got, ds := layoutsOf(t, map[string]string{
		"walker": `{
		  "serves": [{"role": "single", "kind": "planeswalker"}],
		  "stack": [{"part": "bg"}],
		  "textBoxes": {},
		  "sections": {"abilities": {"split": "loyalty", "gap": 10}, "extra": {"split": "loyalty"}}
		}`,
		"walker_tall": `{
		  "extends": ["walker"],
		  "serves": [{"role": "mdfc_back", "kind": "planeswalker"}],
		  "patch": {"sections": {"abilities": {"gap": 20}, "extra": null}}
		}`,
	})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	if s := string(got["walker_tall"]["sections"]); s != `{"abilities":{"gap":20,"split":"loyalty"}}` {
		t.Errorf("sections = %s", s)
	}
	if s := string(got["walker"]["sections"]); !strings.Contains(s, `"extra"`) {
		t.Errorf("the base changed: %s", s)
	}
}
