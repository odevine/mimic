package manifest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/frame"
)

func TestDecodeReadsTheSmallManifestWithoutAProblem(t *testing.T) {
	m, ds := Decode(smallJSON(t))
	if len(ds) != 0 {
		t.Fatalf("diagnostics = %v, want none", ds)
	}
	if m.ID != "mimic.test" || m.Name != "Test frame" || m.License != "CC0-1.0" || m.Author.Name != "mimic" {
		t.Errorf("header = %+v", m)
	}
	if len(m.Credits) != 1 || m.Credits[0].Name != "A. Artist" {
		t.Errorf("credits = %+v", m.Credits)
	}
	if len(m.Parts) != 7 || len(m.Layouts) != 3 || len(m.Features) != 3 {
		t.Errorf("%d parts, %d layouts, %d features, want 7, 3, 3", len(m.Parts), len(m.Layouts), len(m.Features))
	}
	pin := m.Parts["pinlines"]
	if pin.Slot != "pinlines" || pin.Blend == nil || pin.Blend.End != 0.94 || len(pin.Blend.Seams) != 2 || pin.Blend.Seams["2"][0].Width != 650 || len(pin.Blend.Seams["3"]) != 2 {
		t.Errorf("pinlines = %+v", pin)
	}
	if bg := m.Parts["background"]; bg.Origin != (Point{100, 50}) || bg.Variants["gold"].Path != "background/gold.png" {
		t.Errorf("background = %+v", bg)
	}
}

func TestDecodeFillsEntryIDsFromThePartName(t *testing.T) {
	m, _ := Decode(smallJSON(t))
	stack := m.Layouts["normal"].Stack
	var ids []string
	for _, e := range stack {
		ids = append(ids, e.Key())
	}
	if want := []string{"background", "pinlines", "pt", "border", "indicator"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if stack[0].ID != "background" {
		t.Errorf("a decoded entry keeps its filled ID, got %q", stack[0].ID)
	}
}

func TestDecodeKeepsTheLayoutsApart(t *testing.T) {
	m, _ := Decode(smallJSON(t))
	split := m.Layouts["split"]
	if split.Canvas != (Canvas{Width: 4440, Height: 3264, DPI: 1200, Rotate: 270}) {
		t.Errorf("split canvas = %+v", split.Canvas)
	}
	if len(split.Stack) != 2 || split.Stack[0].At == nil || *split.Stack[1].At != (Point{2337, 237}) {
		t.Errorf("split stack = %+v", split.Stack)
	}
	if split.Stack[0].Part != split.Stack[1].Part {
		t.Error("the two halves should use one part")
	}
	if got := m.Layouts["transform"].Serves; len(got) != 2 || got[1] != (frame.Shape{Role: frame.RoleTransformBack, Kind: frame.KindStandard}) {
		t.Errorf("transform serves %+v", got)
	}
	if m.Layouts["split"].TextBoxes["copyright"].Space != "output" {
		t.Error("the output space is lost")
	}
	first, second := split.TextBoxes["title_1"], split.TextBoxes["title_2"]
	if first.Content("title_1") != "title" || second.Content("title_2") != "title" || first.Face != "first" || second.Face != "second" {
		t.Errorf("the two halves' titles = %+v and %+v", first, second)
	}
	if got := split.TextBoxes["copyright"].Content("copyright"); got != "copyright" {
		t.Errorf("a box with no draws draws its key, got %q", got)
	}
}

func TestDecodeMergesEachOverrideOverItsBox(t *testing.T) {
	m, _ := Decode(smallJSON(t))
	title := m.Layouts["normal"].TextBoxes["title"]
	if len(title.When) != 1 || title.When[0].Box == nil {
		t.Fatalf("when = %+v", title.When)
	}
	got := title.When[0].Box
	want := title.Rect
	want.X, want.Width = 514, 1971
	if got.Rect != want {
		t.Errorf("merged rect = %+v, want %+v", got.Rect, want)
	}
	if got.Text != title.Text || got.Flow != title.Flow {
		t.Errorf("an override that changes the rect also changed the rest: %+v", got)
	}
	if title.Rect.X != 386 {
		t.Errorf("the override changed the base box to %+v", title.Rect)
	}
	typ := m.Layouts["normal"].TextBoxes["type"]
	if len(typ.When) != 2 || typ.When[1].Box.Rect.Width != 1800 || typ.When[1].Box.Rect.X != 514 {
		t.Errorf("type overrides = %+v", typ.When)
	}
}

func TestManifestRoundTripsThroughJSON(t *testing.T) {
	m, ds := Decode(smallJSON(t))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	again, ds := Decode(out)
	if len(ds) != 0 {
		t.Fatalf("the encoded manifest reports %v", ds)
	}
	if !reflect.DeepEqual(m, again) {
		t.Error("a decoded manifest changed when it was encoded and decoded again")
	}
}

func TestDecodeReportsTheSameDiagnosticsEveryTime(t *testing.T) {
	data := edit(t, func(r map[string]any) {
		set(t, r, "id", "BAD")
		set(t, r, "layouts.normal.stack[1].part", "nope")
		set(t, r, "layouts.normal.canvas.rotate", 45)
		set(t, r, "parts.background.slot", "border")
	})
	_, first := Decode(data)
	for i := 0; i < 20; i++ {
		if _, again := Decode(data); !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differs:\n%v\n%v", i, first, again)
		}
	}
	if len(first) < 4 {
		t.Errorf("only %d diagnostics: %v", len(first), first)
	}
}

func TestDecodeRefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"not json", []byte(`{"contract": `), "not valid at byte"},
		{"a list", []byte(`[]`), "has to be an object, and it is a list"},
		{"a wrong type", []byte(`{"contract": 1}`), "contract is a number and has to be a string"},
		{"a wrong nested type", []byte(`{"contract": "1.0", "parts": []}`), "parts is a list and has to be an object"},
		{"too large", []byte(`{"contract":"1.0","name":"` + strings.Repeat("a", MaxBytes) + `"}`), "byte limit"},
		{"no contract", []byte(`{}`), "is not a version"},
		{"another major", edit(t, func(r map[string]any) { set(t, r, "contract", "2.0") }), "not one this engine reads"},
		{"a malformed contract", edit(t, func(r map[string]any) { set(t, r, "contract", "1") }), "is not a version"},
	}
	for _, c := range cases {
		m, ds := Decode(c.data)
		if m != nil || !ds.HasErrors() {
			t.Errorf("%s: manifest %v, diagnostics %v, want a refusal", c.name, m != nil, ds)
			continue
		}
		if !strings.Contains(ds.Err().Error(), c.want) {
			t.Errorf("%s: %v does not mention %q", c.name, ds.Err(), c.want)
		}
	}
}

func TestANewerMinorLoadsWithAWarning(t *testing.T) {
	m, ds := Decode(edit(t, func(r map[string]any) { set(t, r, "contract", "1.9") }))
	if m == nil || ds.HasErrors() {
		t.Fatalf("manifest %v, diagnostics %v", m != nil, ds)
	}
	if d, ok := find(ds, "contract"); !ok || d.Severity != Warning {
		t.Errorf("diagnostics = %v, want a warning on contract", ds)
	}
}

func TestDiagnosticsReadAsText(t *testing.T) {
	ds := Diagnostics{
		{Warning, "parts.x", "no layout uses the part"},
		{Error, "layouts.normal.stack[3].part", `no part is named "x"`},
		{Error, "", "the JSON is bad"},
	}
	if !ds.HasErrors() {
		t.Error("HasErrors = false")
	}
	err := ds.Err().Error()
	for _, want := range []string{`error: layouts.normal.stack[3].part: no part is named "x"`, "error: the JSON is bad"} {
		if !strings.Contains(err, want) {
			t.Errorf("Err() = %q, missing %q", err, want)
		}
	}
	if strings.Contains(err, "no layout uses") {
		t.Error("Err() lists a warning")
	}
	if (Diagnostics{{Warning, "a", "b"}}).Err() != nil || (Diagnostics{}).Err() != nil {
		t.Error("Err() should be nil without an error")
	}
}

func TestDecodeStrictRefusesAFieldTheContractLacks(t *testing.T) {
	raw := edit(t, func(root map[string]any) { set(t, root, "surprise", true) })
	if _, ds := Decode(raw); ds.HasErrors() {
		t.Fatalf("a loader refused an unknown field: %v", ds)
	}
	m, ds := DecodeStrict(raw)
	if m != nil || !ds.HasErrors() || !strings.Contains(ds.Err().Error(), "surprise") {
		t.Errorf("strict decode: %v %v", m, ds)
	}
	if _, ds := DecodeStrict(smallJSON(t)); ds.HasErrors() {
		t.Errorf("strict decode refused a good manifest: %v", ds)
	}
}
