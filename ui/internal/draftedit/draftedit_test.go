package draftedit

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/jsonedit"
)

const sourceJSON = `{
  "contract": "1.1",
  "id": "user.test",
  "version": "0.1.0",
  "name": "Test",
  "parts": {
    "bg": {},
    "crown": {},
    "badge": {
      "origin": {
        "x": 7,
        "y": 3
      }
    }
  }
}
`

// layoutJSON is a layout on a 100 by 60 canvas turned by rotate
func layoutJSON(rotate int) string {
	return fmt.Sprintf(`{
  "serves": [
    {
      "role": "single",
      "kind": "standard"
    }
  ],
  "canvas": {
    "width": 100,
    "height": 60,
    "rotate": %d
  },
  "stack": [
    {
      "id": "bg",
      "part": "bg"
    },
    {
      "id": "crown",
      "part": "crown",
      "at": {
        "x": 5,
        "y": 6
      },
      "condition": "legendary"
    },
    {
      "part": "badge"
    }
  ],
  "textBoxes": {
    "title": {
      "rect": {
        "x": 10,
        "y": 5,
        "width": 40,
        "height": 8
      },
      "text": {
        "fontSize": 4
      }
    },
    "legal": {
      "rect": {
        "x": 3,
        "y": 4,
        "width": 20,
        "height": 6
      },
      "space": "output"
    }
  },
  "art": [
    {
      "x": 20,
      "y": 10,
      "width": 30,
      "height": 20,
      "after": "bg"
    }
  ],
  "symbols": {
    "set": {
      "x": 80,
      "y": 40,
      "width": 10,
      "height": 8
    }
  }
}
`, rotate)
}

func draft(rotate int) fstest.MapFS {
	return fstest.MapFS{
		"template.json":       {Data: []byte(sourceJSON)},
		"layouts/normal.json": {Data: []byte(layoutJSON(rotate))},
	}
}

func apply(t *testing.T, fsys fstest.MapFS, ops ...Op) *Result {
	t.Helper()
	res, err := Apply(fsys, ops)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func layoutAfter(t *testing.T, res *Result) *jsonedit.Node {
	t.Helper()
	raw, ok := res.Files["layouts/normal.json"]
	if !ok {
		t.Fatalf("the layout was not changed: %v", res.Files)
	}
	n, err := jsonedit.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func rectOf(t *testing.T, n *jsonedit.Node) image.Rectangle {
	t.Helper()
	r, err := readRect(n)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMovingAnElementChangesOnlyItsNumbers(t *testing.T) {
	fsys := draft(0)
	res := apply(t, fsys, Op{Op: OpMove, Layout: "normal", Kind: KindText, Key: "title", DX: 7, DY: -2})
	got := string(res.Files["layouts/normal.json"])
	want := strings.NewReplacer(`"x": 10,`, `"x": 17,`, `"y": 5,`, `"y": 3,`).Replace(layoutJSON(0))
	if got != want {
		t.Fatalf("the file changed more than the numbers:\n%s", got)
	}
}

func TestEveryKindMoves(t *testing.T) {
	fsys := draft(0)
	res := apply(t, fsys,
		Op{Op: OpMove, Layout: "normal", Kind: KindText, Key: "title", DX: 1, DY: 2},
		Op{Op: OpMove, Layout: "normal", Kind: KindArt, Key: "0", DX: -3, DY: 4},
		Op{Op: OpMove, Layout: "normal", Kind: KindSymbol, Key: "set", DX: 5, DY: -6},
		Op{Op: OpMove, Layout: "normal", Kind: KindLayer, Key: "crown", DX: 10, DY: 10},
	)
	n := layoutAfter(t, res)
	check := func(name string, got, want image.Rectangle) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("title", rectOf(t, n.Path("textBoxes", "title", "rect")), image.Rect(11, 7, 51, 15))
	check("art", rectOf(t, n.Path("art").Item(0)), image.Rect(17, 14, 47, 34))
	check("symbol", rectOf(t, n.Path("symbols", "set")), image.Rect(85, 34, 95, 42))
	at := n.Path("stack").Item(1).Field("at")
	x, _ := at.Field("x").Int()
	y, _ := at.Field("y").Int()
	if x != 15 || y != 16 {
		t.Errorf("crown at %d,%d, want 15,16", x, y)
	}
}

// A layer that places nothing draws at its part's origin, so a move writes the
// position it already has plus the move
func TestMovingALayerWithNoPositionStartsFromItsPartsOrigin(t *testing.T) {
	res := apply(t, draft(0), Op{Op: OpMove, Layout: "normal", Kind: KindLayer, Key: "badge", DX: 2, DY: 1})
	at := layoutAfter(t, res).Path("stack").Item(2).Field("at")
	x, _ := at.Field("x").Int()
	y, _ := at.Field("y").Int()
	if x != 9 || y != 4 {
		t.Errorf("badge at %d,%d, want 9,4 from the origin 7,3", x, y)
	}
	// The entry keeps its other keys, with the new one last
	keys := strings.Join(layoutAfter(t, res).Path("stack").Item(2).Keys(), ",")
	if keys != "part,at" {
		t.Errorf("keys = %s", keys)
	}
}

// A move in the finished image lands the element where the finished image shows it
// moved, whatever the layout's turn
func TestAMoveFollowsTheFinishedImageAcrossRotations(t *testing.T) {
	for _, rotate := range []int{0, 90, 180, 270} {
		for _, d := range []image.Point{{7, 0}, {0, 5}, {-4, 9}} {
			fsys := draft(rotate)
			res := apply(t, fsys, Op{Op: OpMove, Layout: "normal", Kind: KindSymbol, Key: "set", DX: d.X, DY: d.Y})
			n := layoutAfter(t, res)
			before := image.Rect(80, 40, 90, 48)
			after := rectOf(t, n.Path("symbols", "set"))
			shownBefore := template.RotateRect(before, 100, 60, rotate)
			shownAfter := template.RotateRect(after, 100, 60, rotate)
			if got := shownAfter.Min.Sub(shownBefore.Min); got != d {
				t.Errorf("rotate %d, move %v: the symbol moved %v in the finished image", rotate, d, got)
			}
		}
	}
}

func TestOutputSpaceTextBoxesAreMovedInTheFinishedImageDirectly(t *testing.T) {
	res := apply(t, draft(90), Op{Op: OpMove, Layout: "normal", Kind: KindText, Key: "legal", DX: 4, DY: 1})
	got := rectOf(t, layoutAfter(t, res).Path("textBoxes", "legal", "rect"))
	if want := image.Rect(7, 5, 27, 11); got != want {
		t.Errorf("legal = %v, want %v", got, want)
	}
}

func TestResizeTakesARectangleInTheFinishedImage(t *testing.T) {
	for _, rotate := range []int{0, 90, 180, 270} {
		fsys := draft(rotate)
		shown := Rect{X: 12, Y: 9, Width: 17, Height: 11}
		res := apply(t, fsys, Op{Op: OpResize, Layout: "normal", Kind: KindArt, Key: "0", Rect: &shown})
		got := rectOf(t, layoutAfter(t, res).Path("art").Item(0))
		back := template.RotateRect(got, 100, 60, rotate)
		if back != shown.image() {
			t.Errorf("rotate %d: the window is drawn at %v in the finished image, want %v", rotate, back, shown.image())
		}
	}
}

func TestResizeOfAnOutputSpaceBoxIsInTheFinishedImageAlready(t *testing.T) {
	r := Rect{X: 1, Y: 2, Width: 30, Height: 9}
	res := apply(t, draft(90), Op{Op: OpResize, Layout: "normal", Kind: KindText, Key: "legal", Rect: &r})
	if got := rectOf(t, layoutAfter(t, res).Path("textBoxes", "legal", "rect")); got != r.image() {
		t.Errorf("legal = %v, want %v", got, r.image())
	}
}

func TestReorderMovesAnEntryAndLeavesTheRest(t *testing.T) {
	res := apply(t, draft(0), Op{Op: OpReorder, Layout: "normal", Kind: KindLayer, Key: "bg", To: 2})
	stack := layoutAfter(t, res).Path("stack")
	var ids []string
	for i := range stack.Len() {
		e := stack.Item(i)
		id, ok := e.Field("id").String()
		if !ok {
			id, _ = e.Field("part").String()
		}
		ids = append(ids, id)
	}
	if got := strings.Join(ids, ","); got != "crown,badge,bg" {
		t.Errorf("stack = %s", got)
	}
}

func TestEveryOperationHasAnInverseThatRestoresTheFileExactly(t *testing.T) {
	ops := []Op{
		{Op: OpMove, Layout: "normal", Kind: KindText, Key: "title", DX: 13, DY: -7},
		{Op: OpMove, Layout: "normal", Kind: KindLayer, Key: "badge", DX: 3, DY: 3},
		{Op: OpMove, Layout: "normal", Kind: KindLayer, Key: "crown", DX: -2, DY: 8},
		{Op: OpResize, Layout: "normal", Kind: KindSymbol, Key: "set", Rect: &Rect{X: 20, Y: 20, Width: 3, Height: 4}},
		{Op: OpResize, Layout: "normal", Kind: KindText, Key: "legal", Rect: &Rect{X: 2, Y: 3, Width: 9, Height: 9}},
		{Op: OpReorder, Layout: "normal", Kind: KindLayer, Key: "crown", To: 0},
		{Op: OpMove, Layout: "normal", Kind: KindArt, Key: "0", DX: 1, DY: 1},
	}
	for _, rotate := range []int{0, 90, 180, 270} {
		fsys := draft(rotate)
		original := string(fsys["layouts/normal.json"].Data)
		res := apply(t, fsys, ops...)
		if string(res.Files["layouts/normal.json"]) == original {
			t.Fatalf("rotate %d: the operations changed nothing", rotate)
		}
		fsys["layouts/normal.json"] = &fstest.MapFile{Data: res.Files["layouts/normal.json"]}
		undone := apply(t, fsys, res.Inverse...)
		// The badge had no position and now has one, which a move does not take back
		got := strings.ReplaceAll(string(undone.Files["layouts/normal.json"]), `"part": "badge",
      "at": {
        "x": 7,
        "y": 3
      }`, `"part": "badge"`)
		if got != original {
			t.Errorf("rotate %d: undoing the batch did not restore the file:\n%s", rotate, got)
		}
	}
}

func TestOperationsAreRefusedWithTheReason(t *testing.T) {
	size := Rect{X: 0, Y: 0, Width: 5, Height: 5}
	tiny := Rect{Width: 0, Height: 5}
	tests := []struct {
		name string
		op   Op
		want string
	}{
		{"unknown operation", Op{Op: "spin", Layout: "normal", Kind: KindText, Key: "title"}, "not an operation"},
		{"unknown layout", Op{Op: OpMove, Layout: "nope", Kind: KindText, Key: "title"}, `no layout "nope"`},
		{"a layout name that leaves the folder", Op{Op: OpMove, Layout: "../template", Kind: KindText, Key: "title"}, "not a layout"},
		{"missing text box", Op{Op: OpMove, Layout: "normal", Kind: KindText, Key: "gone"}, "no text box"},
		{"missing art", Op{Op: OpMove, Layout: "normal", Kind: KindArt, Key: "4"}, "no art window"},
		{"art by a name", Op{Op: OpMove, Layout: "normal", Kind: KindArt, Key: "main"}, "no art window"},
		{"missing symbol", Op{Op: OpMove, Layout: "normal", Kind: KindSymbol, Key: "x"}, "no symbol"},
		{"missing layer", Op{Op: OpMove, Layout: "normal", Kind: KindLayer, Key: "gone"}, "no entry"},
		{"unknown kind", Op{Op: OpMove, Layout: "normal", Kind: "mana", Key: "x"}, "can be changed"},
		{"resizing a layer", Op{Op: OpResize, Layout: "normal", Kind: KindLayer, Key: "bg", Rect: &size}, "cannot be resized"},
		{"resizing to nothing", Op{Op: OpResize, Layout: "normal", Kind: KindText, Key: "title", Rect: &tiny}, "at least one pixel"},
		{"resizing with no size", Op{Op: OpResize, Layout: "normal", Kind: KindText, Key: "title"}, "at least one pixel"},
		{"reordering text", Op{Op: OpReorder, Layout: "normal", Kind: KindText, Key: "title"}, "only a layer"},
		{"reordering past the top", Op{Op: OpReorder, Layout: "normal", Kind: KindLayer, Key: "bg", To: 9}, "no place 9"},
		{"reordering below the bottom", Op{Op: OpReorder, Layout: "normal", Kind: KindLayer, Key: "bg", To: -1}, "no place -1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Apply(draft(0), []Op{tt.op})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one that mentions %q", err, tt.want)
			}
		})
	}
}

func TestABatchWithABadOperationChangesNothing(t *testing.T) {
	res, err := Apply(draft(0), []Op{
		{Op: OpMove, Layout: "normal", Kind: KindText, Key: "title", DX: 1},
		{Op: OpMove, Layout: "normal", Kind: KindText, Key: "gone", DX: 1},
	})
	if err == nil || res != nil {
		t.Fatalf("got %v, %v", res, err)
	}
	if !strings.Contains(err.Error(), `"gone"`) {
		t.Errorf("the error does not name the operation that failed: %v", err)
	}
}

func TestALayoutThatExtendsAnotherIsRefused(t *testing.T) {
	for _, field := range []string{`"extends": ["base"]`, `"patch": {}`} {
		fsys := draft(0)
		fsys["layouts/normal.json"] = &fstest.MapFile{Data: []byte(`{"serves": [], "canvas": {"width": 10, "height": 10}, ` + field + `}`)}
		_, err := Apply(fsys, []Op{{Op: OpMove, Layout: "normal", Kind: KindText, Key: "title"}})
		if !errors.Is(err, ErrExtends) {
			t.Errorf("%s: error = %v, want ErrExtends", field, err)
		}
	}
}

func TestADraftThatCannotBeReadIsRefused(t *testing.T) {
	broken := draft(0)
	broken["layouts/normal.json"] = &fstest.MapFile{Data: []byte("{")}
	noCanvas := draft(0)
	noCanvas["layouts/normal.json"] = &fstest.MapFile{Data: []byte(`{"serves": []}`)}
	array := draft(0)
	array["layouts/normal.json"] = &fstest.MapFile{Data: []byte(`[]`)}
	noSource := draft(0)
	delete(noSource, "template.json")
	for name, fsys := range map[string]fstest.MapFS{"not JSON": broken, "no canvas": noCanvas, "not an object": array, "no template.json": noSource} {
		op := Op{Op: OpMove, Layout: "normal", Kind: KindLayer, Key: "badge"}
		if _, err := Apply(fsys, []Op{op}); err == nil {
			t.Errorf("%s: a draft that cannot be read was edited", name)
		}
	}
}

func TestAuthoredDeltaIsTheInverseOfTheTurn(t *testing.T) {
	r := image.Rect(10, 20, 30, 35)
	for _, rotate := range []int{0, 90, 180, 270} {
		for _, d := range []image.Point{{3, 0}, {0, 4}, {-5, 7}} {
			ax, ay := authoredDelta(d.X, d.Y, rotate)
			moved := template.RotateRect(r.Add(image.Pt(ax, ay)), 100, 60, rotate)
			base := template.RotateRect(r, 100, 60, rotate)
			if got := moved.Min.Sub(base.Min); got != d {
				t.Errorf("rotate %d: a delivered move of %v is %v in the layout, which shows as %v", rotate, d, image.Pt(ax, ay), got)
			}
		}
	}
}
