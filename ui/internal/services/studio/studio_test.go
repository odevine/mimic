package studio

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/drafts"
	"github.com/odevine/mimic/ui/internal/jobs"
	"github.com/odevine/mimic/ui/internal/workspace"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

func newService(t *testing.T) (*Service, *workspace.Workspace, string) {
	t.Helper()
	ws := workspacetest.NewSmall(t)
	dir := filepath.Join(t.TempDir(), "drafts")
	svc := NewWith(ws, drafts.NewStore(dir))
	t.Cleanup(svc.Shutdown)
	return svc, ws, dir
}

// finish waits for a render job and returns its last event
func finish(t *testing.T, ws *workspace.Workspace, id string) jobs.Event {
	t.Helper()
	j, ok := ws.Jobs.Lookup(id)
	if !ok {
		t.Fatalf("no job %s", id)
	}
	deadline := time.After(30 * time.Second)
	from := 0
	for {
		events, done, wait := j.Stream(from)
		from += len(events)
		if done {
			return events[len(events)-1]
		}
		select {
		case <-wait:
		case <-deadline:
			t.Fatal("the render did not finish")
		}
	}
}

func presetCard(t *testing.T, id string) card.Data {
	t.Helper()
	for _, p := range presets() {
		if p.ID == id {
			return *p.Card.Data
		}
	}
	t.Fatalf("no preset %s", id)
	return card.Data{}
}

// renderDraft renders a card through a draft and returns the image
func renderDraft(t *testing.T, svc *Service, ws *workspace.Workspace, id string, c card.Data) image.Image {
	t.Helper()
	started, err := svc.Render(RenderRequest{ID: id, Card: c})
	if err != nil {
		t.Fatal(err)
	}
	if ev := finish(t, ws, started.JobID); ev.Err != "" {
		t.Fatalf("render failed: %s", ev.Err)
	}
	j, _ := ws.Jobs.Lookup(started.JobID)
	img, _ := j.Result()
	if img == nil {
		t.Fatal("the render left no image")
	}
	return img
}

func samePixels(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}

func TestOpenDescribesALoadableDraft(t *testing.T) {
	svc, _, _ := newService(t)
	sum, err := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.Open(sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Valid || len(v.Layouts) != 1 {
		t.Fatalf("view = %+v", v)
	}
	l := v.Layouts[0]
	if l.Name != "normal" || l.Width != 272 || l.Height != 370 || l.DPI != 100 || len(l.Serves) != 1 {
		t.Errorf("layout = %+v", l)
	}
	for _, d := range v.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("a new draft reports %+v", d)
		}
	}
}

func TestOpenSaysWhyADraftDoesNotLoad(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	if err := os.WriteFile(filepath.Join(dir, sum.ID, "layouts", "normal.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Open(sum.ID)
	if err != nil {
		t.Fatalf("a draft that does not load is shown, not failed: %v", err)
	}
	if v.Valid || len(v.Layouts) != 0 || len(v.Diagnostics) == 0 || v.Diagnostics[0].Severity != "error" {
		t.Fatalf("view = %+v", v)
	}
	if !strings.Contains(v.Diagnostics[0].Path, "normal") {
		t.Errorf("the diagnostic does not point at the layout: %+v", v.Diagnostics[0])
	}
}

func TestOpenRefusesADraftThatIsGone(t *testing.T) {
	svc, _, _ := newService(t)
	if _, err := svc.Open("nothing"); apierr.KindOf(err) != apierr.NotFound {
		t.Fatalf("got %v, want not found", err)
	}
}

func TestRenderDrawsTheTestCardThroughTheDraft(t *testing.T) {
	svc, ws, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	started, err := svc.Render(RenderRequest{ID: sum.ID, Card: presetCard(t, "white-creature")})
	if err != nil {
		t.Fatal(err)
	}
	if started.DPI != 100 {
		t.Errorf("rendered at %d dpi, want the canvas's own 100, since the preview setting is higher", started.DPI)
	}
	if ev := finish(t, ws, started.JobID); ev.Err != "" {
		t.Fatal(ev.Err)
	}
	j, _ := ws.Jobs.Lookup(started.JobID)
	img, name := j.Result()
	if img == nil || img.Bounds().Size() != (image.Point{X: 272, Y: 370}) || name != "Dawnwatch Sentinel" {
		t.Fatalf("image %v, name %q", img.Bounds(), name)
	}
}

func TestRenderSeesAnEditWithoutOpeningAgain(t *testing.T) {
	svc, ws, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	c := presetCard(t, "white-creature")
	before := renderDraft(t, svc, ws, sum.ID, c)

	file := filepath.Join(dir, sum.ID, "layouts", "normal.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var layout map[string]any
	if err := json.Unmarshal(raw, &layout); err != nil {
		t.Fatal(err)
	}
	title := layout["textBoxes"].(map[string]any)["title"].(map[string]any)["text"].(map[string]any)
	title["color"] = "#FF0000"
	edited, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	if samePixels(before, renderDraft(t, svc, ws, sum.ID, c)) {
		t.Fatal("a change to the layout did not reach the next render")
	}
}

func TestRenderReadsAChangedImageAfterOpen(t *testing.T) {
	svc, ws, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	c := presetCard(t, "white-creature")
	before := renderDraft(t, svc, ws, sum.ID, c)

	file := filepath.Join(dir, sum.ID, "parts", "background", "any.png")
	if err := os.WriteFile(file, solid(t, 272, 370), 0o644); err != nil {
		t.Fatal(err)
	}
	if !samePixels(before, renderDraft(t, svc, ws, sum.ID, c)) {
		t.Fatal("the draft read a changed image without being opened again, so its cache is not holding")
	}
	if _, err := svc.Open(sum.ID); err != nil {
		t.Fatal(err)
	}
	if samePixels(before, renderDraft(t, svc, ws, sum.ID, c)) {
		t.Fatal("opening the draft did not start its images afresh")
	}
}

func TestRenderRefusesWhatCannotRender(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	c := presetCard(t, "white-creature")

	if _, err := svc.Render(RenderRequest{ID: "nothing", Card: c}); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("a missing draft gave %v", err)
	}
	if _, err := svc.Render(RenderRequest{ID: sum.ID, Card: c, Face: 4}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a face the card lacks gave %v", err)
	}
	transform := card.Data{Name: "Day // Night", Layout: "transform", TypeLine: "Creature", Faces: []card.Face{{Name: "Day", TypeLine: "Creature"}, {Name: "Night", TypeLine: "Creature"}}}
	_, err := svc.Render(RenderRequest{ID: sum.ID, Card: transform})
	if apierr.KindOf(err) != apierr.BadRequest || !strings.Contains(err.Error(), "does not render") {
		t.Errorf("a card the template does not serve gave %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, sum.ID, "layouts", "normal.json"), []byte(`{"serves": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Render(RenderRequest{ID: sum.ID, Card: c})
	if apierr.KindOf(err) != apierr.BadRequest || !strings.Contains(err.Error(), "normal") {
		t.Errorf("a draft that does not load gave %v, want its first error", err)
	}
}

func TestEveryPresetIsAStandardCardAndRenders(t *testing.T) {
	svc, ws, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	seen := map[string]bool{}
	for _, p := range svc.Presets() {
		if seen[p.ID] || p.Label == "" {
			t.Errorf("preset %+v repeats an id or has no label", p)
		}
		seen[p.ID] = true
		if len(p.Card.Shapes) != 1 || p.Card.Shapes[0].String() != "standard (single)" {
			t.Errorf("%s has shapes %v, want one standard single face", p.ID, p.Card.Shapes)
		}
		renderDraft(t, svc, ws, sum.ID, *p.Card.Data)
	}
	if len(seen) < 8 {
		t.Errorf("only %d presets", len(seen))
	}
}

func TestSetTestCardIsRememberedAndShownOnOpen(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	c := presetCard(t, "two-color")
	if err := svc.SetTestCard(TestCardRequest{ID: sum.ID, Card: &c, Face: 0}); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Open(sum.ID)
	if err != nil || v.Meta.TestCard == nil || v.Meta.TestCard.Name != c.Name {
		t.Fatalf("meta = %+v, %v", v.Meta, err)
	}
	if err := svc.SetTestCard(TestCardRequest{ID: "nothing", Card: &c}); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("remembering for a missing draft gave %v", err)
	}
}

func TestForkCopiesAnInstalledTemplate(t *testing.T) {
	svc, ws, _ := newService(t)
	sum, err := svc.Fork(ForkRequest{Template: workspacetest.ActiveID, Version: "local", Name: "My copy"})
	if err != nil {
		t.Fatal(err)
	}
	if sum.ForkedFrom != workspacetest.ActiveID {
		t.Errorf("forked from %q", sum.ForkedFrom)
	}
	v, err := svc.Open(sum.ID)
	if err != nil || !v.Valid {
		t.Fatalf("view = %+v, %v", v, err)
	}
	renderDraft(t, svc, ws, sum.ID, presetCard(t, "white-creature"))

	if _, err := svc.Fork(ForkRequest{Template: "no.such", Version: "1.0.0", Name: "X"}); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("forking a template that is not installed gave %v", err)
	}
	if list, _ := svc.List(); len(list) != 1 {
		t.Errorf("a failed fork left a draft: %+v", list)
	}
}

func TestDeleteRemovesTheDraftAndItsImages(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	if _, err := svc.Open(sum.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(sum.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, sum.ID)); !os.IsNotExist(err) {
		t.Errorf("the folder is still there: %v", err)
	}
	svc.mu.Lock()
	held := len(svc.open)
	svc.mu.Unlock()
	if held != 0 {
		t.Errorf("%d drafts are still held open", held)
	}
}

// solid is a w by h PNG of one color
func solid(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 0x30, 0x60, 0xA0, 0xFF
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
