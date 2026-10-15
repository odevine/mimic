package pipeline

import (
	"context"
	"image"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/templatetest"
)

func TestOpenSourceReadsABundleAndALooseFolder(t *testing.T) {
	tmp := withEmptyAssetChain(t)
	LooseDirBases = []string{filepath.Join(tmp, "assets")}
	templatetest.Spec{ID: "test.loose"}.WriteLoose(t, filepath.Join(tmp, "assets", "loose"))
	seedBundle(t, templatetest.Spec{ID: "test.cached", Version: "0.2.0", MinEngine: "0.1.0"})

	p, header, closeFn, err := OpenSource("test.cached", "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	if header.Version != "0.2.0" || header.MinEngine != "0.1.0" || header.ID != "test.cached" {
		t.Errorf("header = %+v", header)
	}
	if _, err := p.ReadManifest(); err != nil {
		t.Errorf("the bundle's manifest cannot be read: %v", err)
	}

	p, header, closeFn2, err := OpenSource("test.loose", LocalVersion)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn2()
	if header != (template.BundleHeader{}) {
		t.Errorf("a loose folder has header %+v, want none", header)
	}
	if _, err := p.ReadManifest(); err != nil {
		t.Errorf("the folder's manifest cannot be read: %v", err)
	}
}

func TestOpenSourceRefusesWhatIsNotInstalled(t *testing.T) {
	withEmptyAssetChain(t)
	for _, v := range []string{LocalVersion, "1.0.0"} {
		if _, _, _, err := OpenSource("test.absent", v); err == nil {
			t.Errorf("version %q of a template that is not installed opened", v)
		}
	}
}

// RenderThrough draws with the template it is given and not the one the app uses
func TestRenderThroughUsesTheTemplateItIsGiven(t *testing.T) {
	withEmptyAssetChain(t)
	seedBundle(t, templatetest.Spec{ID: "test.active", Version: "0.1.0"})
	active, _, err := Resolve("test.active")
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	p := New(nil, func() map[string]prefs.TemplateChoice { return nil })
	p.Install(active)

	b, err := templatetest.Spec{ID: "test.draft", Name: "Draft"}.Pack()
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := template.Load(template.NewMemoryAssets(b.ManifestJSON, b.Assets))
	if err != nil {
		t.Fatal(err)
	}
	draft := NewDraftTemplate("test.draft", loaded)
	if draft.Name != "test.draft" || draft.Version != DraftVersion {
		t.Fatalf("draft template = %s %s", draft.Name, draft.Version)
	}

	c := &card.Data{Name: "X", TypeLine: "Creature", Colors: []card.Color{card.White}, Layout: "normal"}
	var steps int
	img, err := p.RenderThrough(context.Background(), draft, c, 0, nil, 0, Visibility{}, func(string, float64) { steps++ })
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got != (image.Point{X: templatetest.Width, Y: templatetest.Height}) {
		t.Errorf("rendered %v", got)
	}
	if steps == 0 {
		t.Error("no progress was reported")
	}
	if _, err := p.RenderThrough(context.Background(), draft, c, 3, nil, 0, Visibility{}, nil); err == nil {
		t.Error("a face the card lacks rendered")
	}
}
