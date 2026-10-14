package studio

import (
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/cardlist"
)

func TestASavedCardJoinsThePresetsWithItsEditsAndResolvesWithoutALookup(t *testing.T) {
	svc, _, _ := newService(t)
	// A lookup that would fail shows that choosing a saved card never makes one
	withSource(svc, &source{fail: apierr.New(apierr.BadGateway, "offline")})
	name := "Edited Name"
	p, err := svc.SavePreset(SavePresetRequest{Label: "Long title", Card: card.Data{Name: "Mirran Spy", SetCode: "MBS", CollectorNumber: "26"}, Edits: &cardlist.Edits{Name: name, SetCode: "MBS", Collector: "26"}})
	if err != nil {
		t.Fatal(err)
	}
	if !p.User || p.ID != "user-long-title" || p.Name != "Edited Name" || p.Set != "mbs" || p.Number != "26" {
		t.Fatalf("preset = %+v", p)
	}
	list := svc.Presets()
	if len(list) != 7 || list[6] != p || list[0].User {
		t.Fatalf("presets = %+v", list)
	}
	got, err := svc.ResolvePreset(p.ID)
	if err != nil || got.Data.Name != "Edited Name" || len(got.Shapes) == 0 {
		t.Fatalf("resolved = %+v, %v", got, err)
	}
}

func TestASavedCardIsRemovedButABuiltInOneIsNot(t *testing.T) {
	svc, _, _ := newService(t)
	p, _ := svc.SavePreset(SavePresetRequest{Label: "Mine", Card: card.Data{Name: "A card"}})
	if err := svc.DeletePreset("land"); err == nil || apierr.KindOf(err) != apierr.BadRequest {
		t.Fatalf("removing a built-in card = %v", err)
	}
	if err := svc.DeletePreset(p.ID); err != nil {
		t.Fatal(err)
	}
	if len(svc.Presets()) != 6 {
		t.Fatalf("presets = %+v", svc.Presets())
	}
	if _, err := svc.ResolvePreset(p.ID); err == nil || apierr.KindOf(err) != apierr.NotFound {
		t.Fatalf("resolving a removed card = %v", err)
	}
}

func TestACardWithNoNameIsNotSaved(t *testing.T) {
	svc, _, _ := newService(t)
	if _, err := svc.SavePreset(SavePresetRequest{Label: "Blank", Card: card.Data{}}); err == nil || apierr.KindOf(err) != apierr.BadRequest {
		t.Fatalf("save = %v", err)
	}
}

func TestACoverageRenderIsSmallerThanAPreviewAndNeedsNoArt(t *testing.T) {
	svc, ws, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "My frame"})
	c := presetCard(t, "monocolored")
	full := renderDraft(t, svc, ws, sum.ID, c)
	// The workspace here has no way to fetch art, so a render that asked for it would fail
	c.ArtworkURL = "http://unreachable.invalid/art.png"
	started, err := svc.Render(RenderRequest{ID: sum.ID, Card: c, Coverage: true})
	if err != nil {
		t.Fatal(err)
	}
	ev := finish(t, ws, started.JobID)
	if ev.Err != "" || ev.ArtMissing {
		t.Fatalf("the coverage render ended with %+v, and it should not have tried the art", ev)
	}
	j, _ := ws.Jobs.Lookup(started.JobID)
	small, _ := j.Result()
	if small == nil || small.Bounds().Dx() >= full.Bounds().Dx() {
		t.Fatalf("a coverage render is %v and a preview is %v", small.Bounds(), full.Bounds())
	}
	if started.DPI >= 100 {
		t.Fatalf("coverage dpi = %d", started.DPI)
	}
}
