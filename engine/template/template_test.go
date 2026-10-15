package template

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

// smallTemplate loads the manifest the manifest package tests against, which uses
// every part of the format
func smallTemplate(t *testing.T) (*Template, *memAssets) {
	t.Helper()
	raw, err := os.ReadFile("../manifest/testdata/small.json")
	if err != nil {
		t.Fatal(err)
	}
	p := &memAssets{manifest: raw}
	tmpl, ds, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v\n%v", err, ds)
	}
	if len(ds) != 0 {
		t.Fatalf("diagnostics = %v, want none", ds)
	}
	return tmpl, p
}

// editedSmall loads the small manifest after f has changed its decoded form
func editedSmall(t *testing.T, f func(m *manifest.Manifest)) (*Template, manifest.Diagnostics, error) {
	t.Helper()
	raw, err := os.ReadFile("../manifest/testdata/small.json")
	if err != nil {
		t.Fatal(err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	f(&m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return Load(&memAssets{manifest: out})
}

func TestLoadCompilesTheLayoutsAndTheShapesTheyServe(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	if tmpl.ID() != "mimic.test" {
		t.Errorf("id = %q", tmpl.ID())
	}
	want := []frame.Shape{
		{Role: frame.RoleSingle, Kind: frame.KindStandard},
		{Role: frame.RoleSplit, Kind: frame.KindStandard},
		{Role: frame.RoleTransformBack, Kind: frame.KindStandard},
		{Role: frame.RoleTransformFront, Kind: frame.KindStandard},
	}
	got := tmpl.Shapes()
	if len(got) != len(want) {
		t.Fatalf("shapes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("shape %d = %v, want %v", i, got[i], want[i])
		}
		if !tmpl.Serves(want[i]) {
			t.Errorf("Serves(%v) = false", want[i])
		}
	}
	if tmpl.Serves(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindPlaneswalker}) {
		t.Error("the template serves a planeswalker it has no layout for")
	}
	l, ok := tmpl.Layout(frame.Shape{Role: frame.RoleTransformBack, Kind: frame.KindStandard})
	if !ok || l.Name != "transform" {
		t.Errorf("the transform back layout = %v, %v", l, ok)
	}
	if _, ok := tmpl.Layout(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindToken}); ok {
		t.Error("a layout was found for a shape nothing serves")
	}
}

func TestShapesIsACopy(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	tmpl.Shapes()[0] = frame.Shape{}
	if tmpl.Shapes()[0] == (frame.Shape{}) {
		t.Error("changing the list Shapes returned changed the template")
	}
}

func TestPrimaryIsTheLayoutForAnOrdinaryCard(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	if got := tmpl.Primary().Name; got != "normal" {
		t.Errorf("Primary = %q, want normal", got)
	}
	only, _, err := editedSmall(t, func(m *manifest.Manifest) {
		delete(m.Layouts, "normal")
		delete(m.Layouts, "split")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := only.Primary().Name; got != "transform" {
		t.Errorf("Primary with no ordinary layout = %q, want the first by name", got)
	}
}

func TestInfoDescribesTheTemplateWithoutDrawingIt(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	info := tmpl.Info()
	if info.ID != "mimic.test" || info.Name != "Test frame" || info.License != "CC0-1.0" || info.Author.Name != "mimic" {
		t.Errorf("info = %+v", info)
	}
	if len(info.Credits) != 1 || info.Credits[0].Name != "A. Artist" || len(info.Shapes) != 4 {
		t.Errorf("credits %v, shapes %v", info.Credits, info.Shapes)
	}
	var names []string
	for _, f := range info.Features {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "ptStyle,textless,texture" {
		t.Errorf("features = %v, want them sorted by name", names)
	}
	if info.Features[2].Type != "range" || info.Features[2].Max != 1 {
		t.Errorf("a feature lost its settings: %+v", info.Features[2])
	}
}

func TestLayoutsKeepTheirOwnCanvasAndEntries(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	split, _ := tmpl.Layout(frame.Shape{Role: frame.RoleSplit, Kind: frame.KindStandard})
	if !split.Turned() || split.DeliveredWidth() != 3264 || split.DeliveredHeight() != 4440 || split.NativeDPI() != 1200 {
		t.Errorf("split canvas %+v delivers %dx%d", split.Canvas, split.DeliveredWidth(), split.DeliveredHeight())
	}
	first, ok := split.Entry("background_1")
	if !ok || first.Origin() != image.Pt(425, 237) || first.Face != "first" {
		t.Errorf("the first half's background = %+v, %v", first, ok)
	}
	second, _ := split.Entry("background_2")
	if second.Origin() != image.Pt(2337, 237) || second.Part != first.Part {
		t.Errorf("the second half = %+v, want the same part placed elsewhere", second)
	}
	normal, _ := tmpl.Layout(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard})
	bg, _ := normal.Entry("background")
	if bg.Origin() != image.Pt(100, 50) {
		t.Errorf("a part's own origin = %v, want 100,50", bg.Origin())
	}
	if _, ok := normal.Entry("nope"); ok {
		t.Error("an entry that does not exist was found")
	}
}

func TestEntriesCompileTheirConditionsModeAndOpacity(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	l, _ := tmpl.Layout(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard})
	border, _ := l.Entry("border")
	if border.Mode.String() == "" || border.Opacity != 0.5 {
		t.Errorf("border mode %v opacity %v", border.Mode, border.Opacity)
	}
	pt, _ := l.Entry("pt")
	if pt.OpacityFeature != "texture" || pt.Opacity != 1 {
		t.Errorf("pt opacity %v from feature %q", pt.Opacity, pt.OpacityFeature)
	}
	creature := frame.Keys{Creature: true}
	if !pt.Condition.Met(creature, frame.Features{"ptStyle": "dark"}) || pt.Condition.Met(creature, frame.Features{"ptStyle": "light"}) || pt.Condition.Met(frame.Keys{}, frame.Features{"ptStyle": "dark"}) {
		t.Error("the pt entry's condition does not follow the card and the feature")
	}
	ind, _ := l.Entry("indicator")
	if ind.Mirror == nil || ind.Mirror.Region != image.Rect(0, 0, 800, 200) {
		t.Errorf("mirror = %+v", ind.Mirror)
	}
	if !ind.Mirror.Condition.Met(frame.Keys{Back: true, TransformIcon: frame.Named("convertdfc")}, nil) {
		t.Error("the mirror condition should hold for a triangle back face")
	}
}

func TestBoxesTryTheOverrideWithTheMostConditionsFirst(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	l, _ := tmpl.Layout(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard})
	typ, ok := l.Box("type")
	if !ok || typ.Draws != "type" || len(typ.Overrides) != 2 {
		t.Fatalf("type box = %+v, %v", typ, ok)
	}
	// "color_indicator,set_symbol" was written second and names two conditions, so it is tried first
	if typ.Overrides[0].Box.Rect.Width != 1800 || typ.Overrides[1].Box.Rect.Width != 1971 {
		t.Errorf("override order = %d, %d, want the two-condition one first", typ.Overrides[0].Box.Rect.Width, typ.Overrides[1].Box.Rect.Width)
	}
	holds := func(k frame.Keys) func(frame.Condition) bool {
		return func(c frame.Condition) bool { return c.Met(k, nil) }
	}
	both := frame.Keys{Indicator: frame.Named("u"), SetSymbol: true}
	if i, ok := typ.Pick(holds(both)); !ok || i != 0 {
		t.Errorf("a card with both = %d, %v, want the first override", i, ok)
	}
	if i, ok := typ.Pick(holds(frame.Keys{Indicator: frame.Named("u")})); !ok || i != 1 {
		t.Errorf("a card with an indicator only = %d, %v, want the second", i, ok)
	}
	if _, ok := typ.Pick(holds(frame.Keys{})); ok {
		t.Error("a plain card took an override")
	}
	if b, ok := l.Box("nope"); ok || b != nil {
		t.Error("a box that does not exist was found")
	}
}

func TestBoxesOfASplitLayoutDrawWhatTheirKeysDoNotSay(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	l, _ := tmpl.Layout(frame.Shape{Role: frame.RoleSplit, Kind: frame.KindStandard})
	if b, _ := l.Box("title_1"); b.Draws != "title" || b.Face != "first" {
		t.Errorf("title_1 = %+v", b)
	}
	if b, _ := l.Box("copyright"); b.Draws != "copyright" || b.Space != "output" {
		t.Errorf("copyright = %+v", b)
	}
	var keys []string
	for _, b := range l.Boxes {
		keys = append(keys, b.Key)
	}
	if strings.Join(keys, ",") != "copyright,title_1,title_2" {
		t.Errorf("boxes = %v, want them sorted by key", keys)
	}
}

func TestPartsAreFoundByNameWhetherOrNotTheStackUsesThem(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	l, _ := tmpl.Layout(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard})
	div, ok := l.Part("divider")
	if !ok || div.Variants["any"] != "divider/any.png" {
		t.Errorf("divider = %+v, %v", div, ok)
	}
	if _, ok := l.Part("nope"); ok {
		t.Error("a part that does not exist was found")
	}
	pin, _ := l.Part("pinlines")
	if pin.Slot != frame.SlotPinlines || pin.Blend == nil || pin.Blend.End != 0.94 {
		t.Errorf("pinlines = %+v", pin)
	}
}

func TestLoadReportsWhyATemplateDidNotLoad(t *testing.T) {
	tmpl, ds, err := editedSmall(t, func(m *manifest.Manifest) {
		m.ID = "BAD"
		e := m.Layouts["normal"].Stack[0]
		e.Part = "nope"
		m.Layouts["normal"].Stack[0] = e
	})
	if tmpl != nil || err == nil {
		t.Fatalf("a bad manifest loaded: %v, %v", tmpl, err)
	}
	if _, ok := diagnosticAt(ds, "id"); !ok {
		t.Errorf("diagnostics = %v, want one at id", ds)
	}
	if _, ok := diagnosticAt(ds, "layouts.normal.stack[0].part"); !ok {
		t.Errorf("diagnostics = %v, want one at the entry's part", ds)
	}
	if !strings.Contains(err.Error(), "layouts.normal.stack[0].part") {
		t.Errorf("error = %v, want it to name the path", err)
	}
}

func diagnosticAt(ds manifest.Diagnostics, path string) (manifest.Diagnostic, bool) {
	for _, d := range ds {
		if d.Path == path {
			return d, true
		}
	}
	return manifest.Diagnostic{}, false
}

func TestLoadReturnsTheWarningsOfATemplateThatLoaded(t *testing.T) {
	tmpl, ds, err := editedSmall(t, func(m *manifest.Manifest) {
		m.Parts["spare"] = manifest.Part{Variants: map[string]manifest.Variant{"any": {Path: "a.png"}}}
	})
	if err != nil || tmpl == nil {
		t.Fatalf("Load = %v, %v", tmpl, err)
	}
	if d, ok := diagnosticAt(ds, "parts.spare"); !ok || d.Severity != manifest.Warning {
		t.Errorf("diagnostics = %v, want a warning for the part nothing uses", ds)
	}
}

func TestLoadRefusesWhatTheEngineDoesNotDrawYet(t *testing.T) {
	cases := map[string]func(m *manifest.Manifest){
		"layouts.normal.stack[0].rotate": func(m *manifest.Manifest) {
			e := m.Layouts["normal"].Stack[0]
			e.Rotate = 90
			m.Layouts["normal"].Stack[0] = e
		},
	}
	for path, edit := range cases {
		tmpl, ds, err := editedSmall(t, edit)
		if tmpl != nil || err == nil {
			t.Errorf("%s: loaded", path)
			continue
		}
		if d, ok := diagnosticAt(ds, path); !ok || !strings.Contains(d.Message, "not drawn by this engine yet") {
			t.Errorf("%s: diagnostics = %v", path, ds)
		}
	}
}

func TestLoadFailsWhenTheProviderCannotReadAManifest(t *testing.T) {
	tmpl, ds, err := Load(&memAssets{})
	if tmpl != nil || err == nil || len(ds) != 0 {
		t.Errorf("Load = %v, %v, %v", tmpl, ds, err)
	}
}

func TestCloseReleasesTheProvider(t *testing.T) {
	tmpl, p := smallTemplate(t)
	if p.closed {
		t.Fatal("the provider was closed by loading")
	}
	if err := tmpl.Close(); err != nil || !p.closed {
		t.Errorf("Close = %v, closed %v", err, p.closed)
	}
}

func TestFeaturesListWhatTheUserCanChoose(t *testing.T) {
	tmpl, _ := smallTemplate(t)
	var seen []string
	for _, f := range tmpl.Features() {
		seen = append(seen, fmt.Sprintf("%s:%s", f.Name, f.Type))
	}
	if strings.Join(seen, " ") != "ptStyle:choice textless:bool texture:range" {
		t.Errorf("features = %v", seen)
	}
}

func TestLibraryAddsGetsListsAndRemoves(t *testing.T) {
	lib := NewLibrary()
	a, _ := smallTemplate(t)
	b, _, err := editedSmall(t, func(m *manifest.Manifest) { m.ID = "mimic.alpha" })
	if err != nil {
		t.Fatal(err)
	}
	if old := lib.Add(a); old != nil {
		t.Errorf("adding to an empty library replaced %v", old)
	}
	lib.Add(b)
	if got, ok := lib.Get("mimic.test"); !ok || got != a {
		t.Errorf("Get = %v, %v", got, ok)
	}
	if _, ok := lib.Get("mimic.none"); ok {
		t.Error("Get found a template that was never added")
	}
	var ids []string
	for _, tm := range lib.List() {
		ids = append(ids, tm.ID())
	}
	if strings.Join(ids, ",") != "mimic.alpha,mimic.test" {
		t.Errorf("List = %v, want them sorted by id", ids)
	}
	if got := lib.Remove("mimic.alpha"); got != b {
		t.Errorf("Remove = %v, want the template removed", got)
	}
	if got := lib.Remove("mimic.alpha"); got != nil {
		t.Errorf("a second Remove returned %v", got)
	}
	if len(lib.List()) != 1 {
		t.Error("the removed template is still listed")
	}
}

func TestLibraryAddReplacesTheTemplateWithTheSameID(t *testing.T) {
	lib := NewLibrary()
	first, _ := smallTemplate(t)
	second, _ := smallTemplate(t)
	lib.Add(first)
	if old := lib.Add(second); old != first {
		t.Errorf("Add returned %v, want the template it replaced", old)
	}
	if got, _ := lib.Get("mimic.test"); got != second || len(lib.List()) != 1 {
		t.Errorf("after replacing: Get = %v, %d templates", got, len(lib.List()))
	}
}

func TestAReplacedTemplateKeepsWorkingForARenderThatHoldsIt(t *testing.T) {
	lib := NewLibrary()
	first, p := smallTemplate(t)
	lib.Add(first)
	held, _ := lib.Get("mimic.test")
	second, _ := smallTemplate(t)
	lib.Add(second)
	lib.Remove("mimic.test")

	shape := frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard}
	l, ok := held.Layout(shape)
	if !ok || l.Geometry(300).Width != 816 {
		t.Error("a template held across a replacement and a removal stopped working")
	}
	if p.closed {
		t.Error("the library closed a template it replaced, which a render may still hold")
	}
}

func TestLibraryServingListsTheTemplatesThatDrawAShape(t *testing.T) {
	lib := NewLibrary()
	full, _ := smallTemplate(t)
	narrow, _, err := editedSmall(t, func(m *manifest.Manifest) {
		m.ID = "mimic.narrow"
		delete(m.Layouts, "split")
		delete(m.Layouts, "transform")
	})
	if err != nil {
		t.Fatal(err)
	}
	lib.Add(full)
	lib.Add(narrow)
	ids := func(sh frame.Shape) string {
		var out []string
		for _, tm := range lib.Serving(sh) {
			out = append(out, tm.ID())
		}
		return strings.Join(out, ",")
	}
	if got := ids(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard}); got != "mimic.narrow,mimic.test" {
		t.Errorf("ordinary cards are drawn by %q", got)
	}
	if got := ids(frame.Shape{Role: frame.RoleSplit, Kind: frame.KindStandard}); got != "mimic.test" {
		t.Errorf("split cards are drawn by %q", got)
	}
	if got := ids(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindSaga}); got != "" {
		t.Errorf("sagas are drawn by %q", got)
	}
}

func TestLibraryIsSafeForConcurrentUse(t *testing.T) {
	lib := NewLibrary()
	base, _ := smallTemplate(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				switch (i + j) % 4 {
				case 0:
					lib.Add(base)
				case 1:
					lib.Remove("mimic.test")
				case 2:
					lib.Get("mimic.test")
				default:
					lib.List()
					lib.Serving(frame.Shape{Role: frame.RoleSingle, Kind: frame.KindStandard})
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestUnsupportedErrorNamesTheTemplateAndTheShape(t *testing.T) {
	err := &UnsupportedError{Template: "mimic.test", Shape: frame.Shape{Role: frame.RoleSingle, Kind: frame.KindPlaneswalker}}
	if got := err.Error(); got != `template "mimic.test" does not render planeswalker (single) faces` {
		t.Errorf("Error = %q", got)
	}
}
