package overrides

import (
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/rules"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

func big() rules.Rule {
	return rules.Rule{
		ID:   "big",
		When: []rules.Condition{{Field: "typeLine", Op: rules.OpContains, Value: "creature"}},
		Then: []rules.Action{{Op: rules.ActionSet, Field: "power", Value: "4"}},
	}
}

func TestRulesRoundTripAndBadOnesAreRefusedWhole(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	if got := svc.Rules(); got == nil || len(got) != 0 {
		t.Fatalf("rules before any = %#v, want an empty list", got)
	}
	kept, err := svc.SaveRules([]rules.Rule{big()})
	if err != nil || len(kept) != 1 || kept[0].ID != "big" {
		t.Fatalf("SaveRules = %+v, %v", kept, err)
	}
	bad := big()
	bad.When[0].Field = "nope"
	if _, err := svc.SaveRules([]rules.Rule{big(), bad}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a list with a bad rule: %v, want a bad request", err)
	}
	if got := svc.Rules(); len(got) != 1 {
		t.Errorf("a refused save changed the stored rules: %+v", got)
	}
}

func TestApplyReportsWhatTheSavedRulesChange(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	svc.SaveRules([]rules.Rule{big()})
	got := svc.Apply(card.Data{Name: "Elves", TypeLine: "Creature — Elf", Power: "1"}, nil)
	if got.Fields["power"] != "4" || len(got.Applied) != 1 {
		t.Errorf("Apply = %+v", got)
	}
	if none := svc.Apply(card.Data{Name: "Bolt", TypeLine: "Instant"}, nil); len(none.Fields) != 0 {
		t.Errorf("a card no rule matches changed: %+v", none)
	}
	// A field the row set itself is left alone, and a condition reads it
	own := svc.Apply(card.Data{Name: "Elves", TypeLine: "Creature — Elf", Power: "1"}, map[string]string{"power": "9"})
	if _, touched := own.Fields["power"]; touched {
		t.Errorf("a rule overwrote a field the row set: %+v", own)
	}
	renamed := svc.Apply(card.Data{Name: "Bolt", TypeLine: "Instant"}, map[string]string{"typeLine": "Creature — Spell"})
	if renamed.Fields["power"] != "4" {
		t.Errorf("a rule did not see the type line the row set: %+v", renamed)
	}
}

func TestMatchesCountsRowsPerRuleCountingTheRowsOwnFields(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	rows := []RowRef{
		{Base: card.Data{TypeLine: "Creature — Elf"}},
		{Base: card.Data{TypeLine: "Instant"}},
		{Base: card.Data{TypeLine: "Instant"}, Fields: map[string]string{"typeLine": "Creature — Spell"}},
	}
	off := big()
	off.ID, off.Disabled = "off", true
	counts, err := svc.Matches([]rules.Rule{big(), off}, rows)
	if err != nil || len(counts) != 2 || counts[0] != 2 || counts[1] != 2 {
		t.Errorf("counts = %v, %v, want 2 and 2, a disabled rule counted as if on", counts, err)
	}
	if _, err := svc.Matches([]rules.Rule{{ID: "x"}}, rows); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("an unfinished rule: %v, want a bad request", err)
	}
}

func TestPresetsSaveApplyAndSayWhichIsCurrent(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	svc.SaveRules([]rules.Rule{big()})
	half := workspacetest.NativeDPI(t, ws) / 2
	ws.Prefs.SetResolution(half, half)
	ws.Prefs.SetSettings(prefs.Settings{Theme: "light", ImageFormat: "png", PNGCompression: "fast", MPCStock: "(S33) Superior Smooth", MPCFoil: true, OutputDir: "/out"})

	saved, err := svc.SavePreset("  House style ")
	if err != nil || saved.Name != "House style" || saved.Rules != 1 || !saved.Current {
		t.Fatalf("SavePreset = %+v, %v", saved, err)
	}

	// Change everything the preset holds, and some of what it does not
	svc.SaveRules(nil)
	ws.Prefs.SetResolution(0, 0)
	ws.Prefs.SetSettings(prefs.Settings{Theme: "dark", ImageFormat: "jpeg", OutputDir: "/elsewhere"})
	if list := svc.Presets(); len(list) != 1 || list[0].Current {
		t.Fatalf("after changes: %+v, want the preset listed and not current", list)
	}

	applied, err := svc.ApplyPreset("house STYLE")
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Rules) != 1 || applied.Settings.ImageFormat != "png" || applied.Settings.PNGCompression != "fast" || !applied.Settings.MPCFoil {
		t.Errorf("applied = %+v", applied)
	}
	if preview, output := ws.Prefs.Resolution(); preview != half || output != half {
		t.Errorf("resolutions = %d and %d, want %d for both", preview, output, half)
	}
	if applied.Settings.Theme != "dark" || applied.Settings.OutputDir != "/elsewhere" {
		t.Errorf("a preset changed settings it does not hold: %+v", applied.Settings)
	}
	if list := svc.Presets(); !list[0].Current {
		t.Errorf("right after applying, %+v, want it current", list)
	}

	if err := svc.DeletePreset("House style"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeletePreset("House style"); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("deleting twice: %v, want not found", err)
	}
	if _, err := svc.ApplyPreset("House style"); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("applying a deleted preset: %v", err)
	}
}

func TestPresetNamesAndVersions(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	if _, err := svc.SavePreset("   "); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a blank name: %v", err)
	}
	ws.Prefs.SetPreset(prefs.Preset{Name: "future", Version: prefs.PresetVersion + 1})
	if _, err := svc.ApplyPreset("future"); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a preset from a newer version: %v, want a bad request", err)
	}
	// Saving under an existing name replaces it, in any case
	svc.SavePreset("Mine")
	svc.SavePreset("MINE")
	if list := svc.Presets(); len(list) != 2 {
		t.Errorf("presets = %+v, want future and one Mine", list)
	}
}
