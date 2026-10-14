package studio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/draftedit"
)

func describeFor(t *testing.T, svc *Service, id, preset string) DescribeView {
	t.Helper()
	v, err := svc.Describe(DescribeRequest{ID: id, Card: presetCard(t, preset)})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func item(t *testing.T, v DescribeView, kind, key string) ItemView {
	t.Helper()
	for _, it := range v.Items {
		if it.Kind == kind && it.Key == key {
			return it
		}
	}
	t.Fatalf("no %s %q in %+v", kind, key, v.Items)
	return ItemView{}
}

func TestDescribeListsWhatTheDraftDrawsForTheTestCard(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	v := describeFor(t, svc, sum.ID, "white-creature")

	if v.Layout != "normal" || v.Authored != (SizeView{272, 370}) || v.Delivered != v.Authored || !v.Editable || v.Reason != "" {
		t.Fatalf("view = %+v", v)
	}
	if bg := item(t, v, "layer", "background"); !bg.Drawn || bg.Delivered != (RectView{0, 0, 272, 370}) {
		t.Errorf("background = %+v", bg)
	}
	if title := item(t, v, "text", "title"); !title.Drawn || title.Delivered.Width == 0 || title.Authored != title.Delivered {
		t.Errorf("title = %+v", title)
	}
	if pt := item(t, v, "text", "pt"); !pt.Drawn {
		t.Errorf("pt = %+v, want drawn for a creature", pt)
	}
	// An art window is there with no art, and says so
	if art := item(t, v, "art", "0"); art.Drawn || art.Why == "" || art.After != "background" {
		t.Errorf("art = %+v", art)
	}
	land := describeFor(t, svc, sum.ID, "land")
	if pt := item(t, land, "text", "pt"); pt.Drawn || pt.Why == "" {
		t.Errorf("a land's pt = %+v, want undrawn with a reason", pt)
	}
}

func TestDescribeSaysWhenALayoutCannotBeEdited(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	layouts := filepath.Join(dir, sum.ID, "layouts")
	raw, _ := os.ReadFile(filepath.Join(layouts, "normal.json"))
	// The starter's layout becomes the base for another shape, and the layout the
	// card renders with builds on it
	base := strings.Replace(string(raw), `"role": "single"`, `"role": "split"`, 1)
	if err := os.WriteFile(filepath.Join(layouts, "base.json"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layouts, "normal.json"), []byte(`{"serves": [{"role": "single", "kind": "standard"}], "extends": ["base"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Describe(DescribeRequest{ID: sum.ID, Card: presetCard(t, "white-creature")})
	if err != nil {
		t.Fatal(err)
	}
	if v.Layout != "normal" || v.Editable || !strings.Contains(v.Reason, "extends") {
		t.Fatalf("view = %+v", v)
	}
	if _, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: []draftedit.Op{{Op: draftedit.OpMove, Layout: "normal", Kind: draftedit.KindText, Key: "title", DX: 1}}}); err == nil || !strings.Contains(err.Error(), "extends") {
		t.Errorf("an edit to a layout that extends another gave %v", err)
	}
}

func TestApplyMovesAnElementSavesItAndRendersFromTheNewFile(t *testing.T) {
	svc, ws, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	c := presetCard(t, "white-creature")
	before := renderDraft(t, svc, ws, sum.ID, c)
	was := describeFor(t, svc, sum.ID, "white-creature")
	title := item(t, was, "text", "title")

	res, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: []draftedit.Op{{Op: draftedit.OpMove, Layout: "normal", Kind: draftedit.KindText, Key: "title", DX: 20, DY: 15}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rev != 1 || len(res.Inverse) != 1 {
		t.Fatalf("result = %+v", res)
	}
	now := describeFor(t, svc, sum.ID, "white-creature")
	moved := item(t, now, "text", "title")
	if moved.Delivered.X != title.Delivered.X+20 || moved.Delivered.Y != title.Delivered.Y+15 || now.Rev != 1 {
		t.Errorf("title moved from %+v to %+v at rev %d", title.Delivered, moved.Delivered, now.Rev)
	}
	if samePixels(before, renderDraft(t, svc, ws, sum.ID, c)) {
		t.Error("the next render did not show the move")
	}
	if _, err := os.Stat(filepath.Join(dir, sum.ID, "layouts", "normal.json")); err != nil {
		t.Fatal(err)
	}

	// The inverse puts it back, and the file reads as it did
	if _, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: res.Inverse}); err != nil {
		t.Fatal(err)
	}
	if back := item(t, describeFor(t, svc, sum.ID, "white-creature"), "text", "title"); back.Delivered != title.Delivered {
		t.Errorf("after undo the title is at %+v, want %+v", back.Delivered, title.Delivered)
	}
	if !samePixels(before, renderDraft(t, svc, ws, sum.ID, c)) {
		t.Error("undoing the move did not restore the render")
	}
}

func TestApplyRefusesWhatCannotBeDoneAndChangesNothing(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	file := filepath.Join(dir, sum.ID, "layouts", "normal.json")
	original, _ := os.ReadFile(file)
	move := func(key string, dx int) ApplyRequest {
		return ApplyRequest{ID: sum.ID, Ops: []draftedit.Op{{Op: draftedit.OpMove, Layout: "normal", Kind: draftedit.KindText, Key: key, DX: dx}}}
	}

	for name, req := range map[string]ApplyRequest{
		"nothing to do":            {ID: sum.ID},
		"a text box that is gone":  move("gone", 1),
		"a draft that is gone":     {ID: "gone", Ops: move("title", 1).Ops},
		"a move off the canvas":    move("title", 10_000_000),
		"a layout that is not one": {ID: sum.ID, Ops: []draftedit.Op{{Op: draftedit.OpMove, Layout: "../template", Kind: draftedit.KindText, Key: "title"}}},
	} {
		_, err := svc.Apply(req)
		if err == nil {
			t.Errorf("%s: the change was made", name)
		} else if k := apierr.KindOf(err); k != apierr.BadRequest && k != apierr.NotFound {
			t.Errorf("%s: error kind %v: %v", name, k, err)
		}
	}
	if now, _ := os.ReadFile(file); string(now) != string(original) {
		t.Error("a refused change altered the layout")
	}
	if svc.rev(sum.ID) != 0 {
		t.Errorf("a refused change counted as a revision: %d", svc.rev(sum.ID))
	}
}

func TestApplyAllowsAnEditToADraftThatAlreadyDoesNotLoad(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	// A broken description does not stop a template loading, but a broken layout
	// does, and a draft in that state is for the JSON to fix, not for an edit to
	// refuse over a problem it did not cause
	tpl := filepath.Join(dir, sum.ID, "template.json")
	raw, _ := os.ReadFile(tpl)
	if err := os.WriteFile(tpl, []byte(strings.Replace(string(raw), `"contract": "1.1"`, `"contract": "9.0"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: []draftedit.Op{{Op: draftedit.OpMove, Layout: "normal", Kind: draftedit.KindText, Key: "title", DX: 1}}})
	if err != nil {
		t.Fatalf("an edit to a draft that did not load was refused: %v", err)
	}
}
