package templates

import (
	"testing"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
)

// oneTemplate builds a one-template index for the row tests
func oneTemplate(name string, versions ...catalog.Version) *catalog.Index {
	return &catalog.Index{Schema: 1, Templates: []catalog.Template{{
		Name:     name,
		Latest:   versions[0].Version,
		Versions: versions,
	}}}
}

func findRow(rows []templateRow, name string) (templateRow, bool) {
	for _, r := range rows {
		if r.name == name {
			return r, true
		}
	}
	return templateRow{}, false
}

func findVer(rows []versionRow, version string) (versionRow, bool) {
	for _, r := range rows {
		if r.version == version {
			return r, true
		}
	}
	return versionRow{}, false
}

// noneCached stands for a cache with nothing in it, whatever this computer holds
func noneCached(string) []string { return nil }

func installedInfo(id, desc string, shapes ...template.Shape) pipeline.Installed {
	return pipeline.Installed{ID: id, Info: template.Info{ID: id, Description: desc, Shapes: shapes}}
}

func TestBuildTemplateRows(t *testing.T) {
	// Engine says 0.3.0 satisfies >=0.3.0 but not >=0.4.0
	old := templateVersionSatisfied
	templateVersionSatisfied = func(minEngine string) bool { return minEngine != "0.4.0" }
	defer func() { templateVersionSatisfied = old }()

	idx := &catalog.Index{Schema: 1, Templates: []catalog.Template{
		{Name: "mimic.m15", Description: "from the catalog", Latest: "0.2.0", Versions: []catalog.Version{
			{Version: "0.2.0", Format: 2, MinEngine: "0.4.0"}, // incompatible
			{Version: "0.1.0", Format: 2, MinEngine: "0.3.0"}, // compatible
		}},
		{Name: "mimic.old", Latest: "1.0.0", Versions: []catalog.Version{
			{Version: "1.0.0", Format: 1, MinEngine: "0.3.0"}, // a format this engine does not read
		}},
		{Name: "mimic.walkers", Title: "Walker frames", Shapes: []template.Shape{{Role: "single", Kind: "planeswalker"}}, Latest: "1.0.0", Versions: []catalog.Version{
			{Version: "1.0.0", Format: 2, MinEngine: "0.3.0", Size: 41_300_000},
		}},
	}}
	installed := []pipeline.Installed{installedInfo("mimic.m15", "from the installed copy", pipeline.PrimaryShape)}
	hasLocal := func(n string) bool { return false }
	isCached := func(n, v string) bool { return n == "mimic.m15" && v == "0.1.0" }

	rows := buildTemplateRows(idx, installed, hasLocal, isCached, noneCached)

	row, ok := findRow(rows, "mimic.m15")
	if !ok {
		t.Fatalf("mimic.m15 row missing: %+v", rows)
	}
	if len(row.shapes) != 1 || row.shapes[0] != pipeline.PrimaryShape {
		t.Errorf("shapes = %v, want those of the installed copy", row.shapes)
	}
	if row.description != "from the installed copy" || !row.standard {
		t.Errorf("row = %+v, want the installed copy's description and standard", row)
	}
	if v, _ := findVer(row.versions, "0.1.0"); !v.selectable || !v.cached {
		t.Errorf("0.1.0 = %+v, want selectable and cached", v)
	}
	if v, _ := findVer(row.versions, "0.2.0"); v.selectable || v.reason == "" {
		t.Errorf("0.2.0 = %+v, want not selectable with a reason", v)
	}
	if _, ok := findRow(rows, "mimic.old"); ok {
		t.Error("a template with only format 1 bundles has a row")
	}
	walkers, ok := findRow(rows, "mimic.walkers")
	if !ok || walkers.standard {
		t.Errorf("walkers = %+v, want a row that does not render standard cards", walkers)
	}
	if walkers.title != "Walker frames" {
		t.Errorf("walkers title = %q, want the catalog's", walkers.title)
	}
	if len(walkers.shapes) != 1 || walkers.shapes[0].Kind != "planeswalker" {
		t.Errorf("walkers shapes = %v, want the catalog's", walkers.shapes)
	}
	if v, _ := findVer(walkers.versions, "1.0.0"); !v.selectable || v.cached || v.size != 41_300_000 {
		t.Errorf("walkers 1.0.0 = %+v, want selectable and not cached", v)
	}
}

func TestBuildTemplateRowsAddsLocalRow(t *testing.T) {
	idx := oneTemplate("mimic.m15", catalog.Version{Version: "0.1.0", Format: 2, MinEngine: "0.3.0"})
	rows := buildTemplateRows(idx, []pipeline.Installed{installedInfo("mimic.m15", "", pipeline.PrimaryShape)},
		func(n string) bool { return n == "mimic.m15" },
		func(n, v string) bool { return false }, noneCached)

	row, ok := findRow(rows, "mimic.m15")
	if !ok {
		t.Fatal("mimic.m15 row missing")
	}
	if len(row.versions) == 0 || row.versions[0].version != pipeline.LocalVersion {
		t.Fatalf("first version = %+v, want the synthetic local row", row.versions)
	}
	if !row.versions[0].selectable {
		t.Error("local row should be selectable")
	}
}

// An installed template with local assets but no catalog entry still appears
func TestBuildTemplateRowsLocalOnlyTemplate(t *testing.T) {
	rows := buildTemplateRows(nil, []pipeline.Installed{installedInfo("mimic.m15", "mine", pipeline.PrimaryShape)},
		func(n string) bool { return n == "mimic.m15" },
		func(n, v string) bool { return false }, noneCached)

	row, ok := findRow(rows, "mimic.m15")
	if !ok || len(row.versions) != 1 || row.versions[0].version != pipeline.LocalVersion || row.description != "mine" {
		t.Fatalf("local-only row = %+v, want a single local version", row)
	}
}

func TestVersionLabelNamesTheVersionsThatAreNotReleases(t *testing.T) {
	cases := map[string]string{
		pipeline.LocalVersion: "local (developer assets)",
		pipeline.DraftVersion: "draft (from the editor)",
		"1.2.3":               "1.2.3",
	}
	for in, want := range cases {
		if got := versionLabel(in); got != want {
			t.Errorf("versionLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestATemplateTheEditorMadeIsListedAtTheDraftVersion(t *testing.T) {
	in := pipeline.Installed{ID: "user.mine", Info: template.Info{Name: "Mine"}}
	rows := buildTemplateRows(nil, []pipeline.Installed{in}, func(string) bool { return false }, func(string, string) bool { return true }, func(string) []string { return []string{pipeline.DraftVersion} })
	row, ok := findRow(rows, "user.mine")
	if !ok || len(row.versions) != 1 || row.versions[0].version != "draft" || !row.versions[0].selectable || !row.versions[0].cached {
		t.Fatalf("row = %+v, %v", row, ok)
	}
}
