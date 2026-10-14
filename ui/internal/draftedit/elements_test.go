package draftedit

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/jsonedit"
)

// held is Blobs in memory
type held map[string][]byte

func (h held) Get(name string) ([]byte, bool) { b, ok := h[name]; return b, ok }
func (h held) Put(data []byte) string {
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:6])
	h[name] = data
	return name
}

func applyWith(t *testing.T, fsys fstest.MapFS, blobs Blobs, ops ...Op) *Result {
	t.Helper()
	res, err := ApplyWith(fsys, ops, blobs)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// write puts a result's changes into a copy of the folder, as the service saves them
func write(fsys fstest.MapFS, res *Result) fstest.MapFS {
	out := fstest.MapFS{}
	for k, v := range fsys {
		out[k] = v
	}
	for name, data := range res.Files {
		out[name] = &fstest.MapFile{Data: data}
	}
	for _, name := range res.Remove {
		delete(out, name)
	}
	return out
}

func same(t *testing.T, a, b fstest.MapFS) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("the folders hold %d and %d files", len(a), len(b))
	}
	for name, f := range a {
		g, ok := b[name]
		if !ok || string(f.Data) != string(g.Data) {
			t.Errorf("%s differs", name)
		}
	}
}

// roundTrip applies ops and then their inverse, and expects the folder as it began
func roundTrip(t *testing.T, fsys fstest.MapFS, blobs Blobs, ops ...Op) fstest.MapFS {
	t.Helper()
	res := applyWith(t, fsys, blobs, ops...)
	after := write(fsys, res)
	back := applyWith(t, after, blobs, res.Inverse...)
	same(t, fsys, write(after, back))
	return after
}

func TestAddingATextBoxPlacesItInTheFinishedImage(t *testing.T) {
	for _, rotate := range []int{0, 90, 180, 270} {
		fsys := draft(rotate)
		want := Rect{X: 12, Y: 9, Width: 30, Height: 7}
		after := roundTrip(t, fsys, nil, Op{Op: OpAdd, Layout: "normal", Kind: KindText, Key: "flavor", Value: raw(`{"draws":"flavor","text":{"fontSize":3}}`), Rect: &want})
		l := layoutAfter(t, &Result{Files: map[string][]byte{"layouts/normal.json": after["layouts/normal.json"].Data}})
		stored := rectOf(t, l.Path("textBoxes", "flavor", "rect"))
		// What is written is in the layout's own canvas, 100 by 60, and the turn takes it to the finished image
		got := template.RotateRect(stored, 100, 60, rotate)
		if got != want.image() {
			t.Errorf("rotate %d: stored %v is %v in the finished image, want %v", rotate, stored, got, want.image())
		}
		if draws, _ := l.Path("textBoxes", "flavor", "draws").String(); draws != "flavor" {
			t.Errorf("the box lost its fields: %s", l.Path("textBoxes", "flavor").Compact())
		}
	}
}

func TestAddingALayerAndAnArtWindow(t *testing.T) {
	fsys := draft(0)
	after := roundTrip(t, fsys, nil,
		Op{Op: OpAdd, Layout: "normal", Kind: KindLayer, Key: "glow", To: 1, Value: raw(`{"id":"glow","part":"crown","opacity":0.5}`)},
		Op{Op: OpAdd, Layout: "normal", Kind: KindArt, Key: "1", Value: raw(`{"after":"glow"}`), Rect: &Rect{X: 1, Y: 2, Width: 10, Height: 12}},
	)
	n := layoutAfter(t, &Result{Files: map[string][]byte{"layouts/normal.json": after["layouts/normal.json"].Data}})
	if id, _ := n.Path("stack").Item(1).Field("id").String(); id != "glow" {
		t.Errorf("the layer is not second in the stack: %s", n.Path("stack").Compact())
	}
	if r := rectOf(t, n.Field("art").Item(1)); r != image.Rect(1, 2, 11, 14) {
		t.Errorf("art window = %v", r)
	}
}

func TestAddIsRefusedWithTheReason(t *testing.T) {
	fsys := draft(0)
	for _, tt := range []struct {
		name string
		op   Op
		why  string
	}{
		{"a layer named for one that is there", Op{Op: OpAdd, Layout: "normal", Kind: KindLayer, Key: "crown", Value: raw(`{"id":"crown","part":"crown"}`)}, "already has"},
		{"a layer with no part", Op{Op: OpAdd, Layout: "normal", Kind: KindLayer, Key: "x", Value: raw(`{"id":"x"}`)}, "part"},
		{"a layer whose id is another name", Op{Op: OpAdd, Layout: "normal", Kind: KindLayer, Key: "x", Value: raw(`{"id":"y","part":"bg"}`)}, "id"},
		{"a layer past the end of the stack", Op{Op: OpAdd, Layout: "normal", Kind: KindLayer, Key: "x", To: 9, Value: raw(`{"id":"x","part":"bg"}`)}, "no place"},
		{"a text box that is there", Op{Op: OpAdd, Layout: "normal", Kind: KindText, Key: "title", Value: raw(`{}`)}, "already has"},
		{"a text box with a bad name", Op{Op: OpAdd, Layout: "normal", Kind: KindText, Key: "a b", Value: raw(`{}`)}, "not a name"},
		{"a symbol", Op{Op: OpAdd, Layout: "normal", Kind: KindSymbol, Key: "x", Value: raw(`{}`)}, "cannot be added"},
		{"an art window past the list", Op{Op: OpAdd, Layout: "normal", Kind: KindArt, Key: "5", Value: raw(`{}`)}, "no place"},
		{"something that is not JSON", Op{Op: OpAdd, Layout: "normal", Kind: KindText, Key: "x", Value: raw(`{`)}, "JSON"},
		{"something that is not an object", Op{Op: OpAdd, Layout: "normal", Kind: KindText, Key: "x", Value: raw(`[]`)}, "object"},
	} {
		if _, err := Apply(fsys, []Op{tt.op}); err == nil || !strings.Contains(err.Error(), tt.why) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.why)
		}
	}
}

func TestDeletingTakesAnElementOutAndTheInverseRestoresTheFile(t *testing.T) {
	for _, op := range []Op{
		{Op: OpDelete, Layout: "normal", Kind: KindLayer, Key: "crown"},
		{Op: OpDelete, Layout: "normal", Kind: KindLayer, Key: "badge"},
		{Op: OpDelete, Layout: "normal", Kind: KindText, Key: "legal"},
		{Op: OpDelete, Layout: "normal", Kind: KindArt, Key: "0"},
	} {
		after := roundTrip(t, draft(0), nil, op)
		if string(after["layouts/normal.json"].Data) == layoutJSON(0) {
			t.Errorf("%s %s changed nothing", op.Kind, op.Key)
		}
	}
	if _, err := Apply(draft(0), []Op{{Op: OpDelete, Layout: "normal", Kind: KindText, Key: "nope"}}); err == nil {
		t.Error("deleted a text box that is not there")
	}
	if _, err := Apply(draft(0), []Op{{Op: OpDelete, Layout: "normal", Kind: KindSymbol, Key: "set"}}); err == nil {
		t.Error("deleted a symbol")
	}
}

func TestDuplicatingACopyIsMovedAndInverseDeletesIt(t *testing.T) {
	fsys := draft(0)
	after := roundTrip(t, fsys, nil,
		Op{Op: OpDuplicate, Layout: "normal", Kind: KindLayer, Key: "crown", As: "crown2", DX: 4, DY: 3},
		Op{Op: OpDuplicate, Layout: "normal", Kind: KindText, Key: "title", As: "title2", DX: 0, DY: 20},
		Op{Op: OpDuplicate, Layout: "normal", Kind: KindArt, Key: "0", DX: 6, DY: 6},
	)
	n := layoutAfter(t, &Result{Files: map[string][]byte{"layouts/normal.json": after["layouts/normal.json"].Data}})
	stack := n.Path("stack")
	if id, _ := stack.Item(2).Field("id").String(); id != "crown2" {
		t.Errorf("the copy is not just above the original: %s", stack.Compact())
	}
	x, _ := stack.Item(2).Path("at", "x").Int()
	y, _ := stack.Item(2).Path("at", "y").Int()
	if x != 9 || y != 9 {
		t.Errorf("the layer copy is at %d,%d, want 9,9", x, y)
	}
	if d, _ := n.Path("textBoxes", "title2", "draws").String(); d != "title" {
		t.Errorf("the copy of a box with no draws draws %q, want title", d)
	}
	if n.Path("textBoxes", "title", "draws") != nil {
		t.Error("the original was given a draws")
	}
	if r := rectOf(t, n.Path("textBoxes", "title2", "rect")); r != image.Rect(10, 25, 50, 33) {
		t.Errorf("title copy = %v", r)
	}
	if r := rectOf(t, n.Path("textBoxes", "title", "rect")); r != image.Rect(10, 5, 50, 13) {
		t.Errorf("the original title moved to %v", r)
	}
	if r := rectOf(t, n.Field("art").Item(1)); r != image.Rect(26, 16, 56, 36) {
		t.Errorf("art copy = %v", r)
	}
	if _, err := Apply(fsys, []Op{{Op: OpDuplicate, Layout: "normal", Kind: KindText, Key: "title", As: "legal"}}); err == nil {
		t.Error("a copy took the name of a text box that is there")
	}
}

func TestADuplicateInARotatedLayoutMovesInTheFinishedImage(t *testing.T) {
	fsys := draft(90)
	res := applyWith(t, fsys, nil, Op{Op: OpDuplicate, Layout: "normal", Kind: KindText, Key: "title", As: "t2", DX: 10, DY: 0})
	n := layoutAfter(t, res)
	was := rectOf(t, n.Path("textBoxes", "title", "rect"))
	now := rectOf(t, n.Path("textBoxes", "t2", "rect"))
	// A turn of 90 sends a step to the right in the finished image to a step along the canvas's own y
	if now.Min.X != was.Min.X || now.Min.Y != was.Min.Y-10 {
		t.Errorf("copy at %v from %v", now, was)
	}
}

func TestPutWritesAnImageAndItsInverseDropsOrRestoresIt(t *testing.T) {
	fsys := draft(0)
	fsys["parts/bg/any.png"] = &fstest.MapFile{Data: []byte("old")}
	blobs := held{}
	fresh := blobs.Put([]byte("new image"))

	res := applyWith(t, fsys, blobs, Op{Op: OpPut, File: "parts/bg/w.png", Stash: fresh})
	if string(res.Files["parts/bg/w.png"]) != "new image" {
		t.Fatalf("files = %v", res.Files)
	}
	if len(res.Inverse) != 1 || res.Inverse[0].Op != OpDrop {
		t.Errorf("the inverse of a new file is %+v", res.Inverse)
	}

	res = applyWith(t, fsys, blobs, Op{Op: OpPut, File: "parts/bg/any.png", Stash: fresh})
	if len(res.Inverse) != 1 || res.Inverse[0].Op != OpPut {
		t.Fatalf("the inverse of a replaced file is %+v", res.Inverse)
	}
	if got, _ := blobs.Get(res.Inverse[0].Stash); string(got) != "old" {
		t.Errorf("the old image was not kept: %q", got)
	}
	roundTrip(t, fsys, blobs, Op{Op: OpPut, File: "parts/bg/any.png", Stash: fresh})
	roundTrip(t, fsys, blobs, Op{Op: OpPut, File: "parts/bg/w.png", Stash: fresh})
}

func TestDropRemovesAFileAndItsInverseWritesItBack(t *testing.T) {
	fsys := draft(0)
	fsys["parts/bg/any.png"] = &fstest.MapFile{Data: []byte("old")}
	blobs := held{}
	res := applyWith(t, fsys, blobs, Op{Op: OpDrop, File: "parts/bg/any.png"})
	if len(res.Remove) != 1 || res.Remove[0] != "parts/bg/any.png" {
		t.Errorf("remove = %v", res.Remove)
	}
	roundTrip(t, fsys, blobs, Op{Op: OpDrop, File: "parts/bg/any.png"})
	if _, err := ApplyWith(fsys, []Op{{Op: OpDrop, File: "parts/bg/none.png"}}, blobs); err == nil {
		t.Error("dropped a file that is not there")
	}
}

func TestAnImageAndAFieldAreChangedTogether(t *testing.T) {
	fsys := draft(0)
	blobs := held{}
	token := blobs.Put([]byte("png"))
	res := applyWith(t, fsys, blobs,
		Op{Op: OpPut, File: "parts/glow/any.png", Stash: token},
		Op{Op: OpSet, Kind: KindTemplate, Path: []string{"parts", "glow"}, Value: raw(`{"dir":"parts/glow"}`)},
	)
	if _, ok := res.Files["parts/glow/any.png"]; !ok {
		t.Error("the image is not written")
	}
	if _, ok := res.Files["template.json"]; !ok {
		t.Error("template.json is not written")
	}
	roundTrip(t, fsys, blobs,
		Op{Op: OpPut, File: "parts/glow/any.png", Stash: token},
		Op{Op: OpSet, Kind: KindTemplate, Path: []string{"parts", "glow"}, Value: raw(`{"dir":"parts/glow"}`)},
	)
}

func TestFilesAreWrittenOnlyWhereTheEditorMay(t *testing.T) {
	blobs := held{}
	token := blobs.Put([]byte("x"))
	for _, file := range []string{"template.json", "layouts/normal.json", "../x.png", "/x.png", ".hidden/x.png", "parts/.x.png", "notes.txt", "parts/x.ttf", "font/sub/x.ttf", "", "."} {
		if _, err := ApplyWith(draft(0), []Op{{Op: OpPut, File: file, Stash: token}}, blobs); err == nil {
			t.Errorf("put %q was allowed", file)
		}
	}
	for _, file := range []string{"parts/bg/any.png", "font/Face.ttf", "font/Face.OTF", "x.png"} {
		if _, err := ApplyWith(draft(0), []Op{{Op: OpPut, File: file, Stash: token}}, blobs); err != nil {
			t.Errorf("put %q was refused: %v", file, err)
		}
	}
	if _, err := Apply(draft(0), []Op{{Op: OpPut, File: "parts/x.png", Stash: token}}); err == nil {
		t.Error("a put with nothing to hold the file was allowed")
	}
	if _, err := ApplyWith(draft(0), []Op{{Op: OpPut, File: "parts/x.png", Stash: "gone"}}, blobs); err == nil || !strings.Contains(err.Error(), "no longer held") {
		t.Errorf("an unknown stash gave %v", err)
	}
}

func TestADeletedTextBoxComesBackWhereItWas(t *testing.T) {
	fsys := draft(0)
	// title is first and legal last, so both ends are covered
	for _, key := range []string{"title", "legal"} {
		after := roundTrip(t, fsys, nil, Op{Op: OpDelete, Layout: "normal", Kind: KindText, Key: key})
		if strings.Contains(string(after["layouts/normal.json"].Data), `"`+key+`"`) {
			t.Errorf("%s is still in the layout", key)
		}
	}
}

func TestAnEditThatGivesATemplateFontsOrRolesNamesTheEngineThatReadsThem(t *testing.T) {
	fsys := draft(0)
	set := Op{Op: OpSet, Kind: KindTemplate, Path: []string{"roles", "body-italic"}, Value: raw(`{"builtin":"mimic-m15-title"}`)}
	after := roundTrip(t, fsys, nil, set)
	src, _ := jsonedit.Parse(after["template.json"].Data)
	if v, _ := src.Field("minEngine").String(); v != MinEngineForFonts {
		t.Fatalf("minEngine = %q in %s", v, after["template.json"].Data)
	}
	// A font does the same, and a template that names a release already keeps it
	font := Op{Op: OpSet, Kind: KindTemplate, Path: []string{"fonts", "face"}, Value: raw(`{"file":"font/face.ttf"}`)}
	if res := applyWith(t, fsys, nil, font); !strings.Contains(string(res.Files["template.json"]), `"minEngine": "`+MinEngineForFonts+`"`) {
		t.Errorf("a font did not name the engine: %s", res.Files["template.json"])
	}
	named := draft(0)
	named["template.json"] = &fstest.MapFile{Data: []byte(strings.Replace(sourceJSON, `"name": "Test",`, `"name": "Test",
  "minEngine": "0.20.0",`, 1))}
	res := applyWith(t, named, nil, set)
	if !strings.Contains(string(res.Files["template.json"]), `"minEngine": "0.20.0"`) {
		t.Errorf("a release the template named was changed: %s", res.Files["template.json"])
	}
	// Editing a layout, or setting nothing that defines a font or a role, names nothing
	if res := applyWith(t, fsys, nil, Op{Op: OpMove, Layout: "normal", Kind: KindText, Key: "title", DX: 1}); res.Files["template.json"] != nil {
		t.Error("a move changed template.json")
	}
	other := Op{Op: OpSet, Kind: KindTemplate, Path: []string{"description"}, Value: raw(`"x"`)}
	if res := applyWith(t, fsys, nil, other); strings.Contains(string(res.Files["template.json"]), "minEngine") {
		t.Error("a description named an engine")
	}
}
