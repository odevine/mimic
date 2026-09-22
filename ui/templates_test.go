package main

import "testing"

// catalog builds a one-template index for the row tests
func catalog(name string, versions ...catalogVersion) *index {
	return &index{Schema: 1, Templates: []catalogTemplate{{
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

func TestBuildTemplateRows(t *testing.T) {
	// Engine says 0.3.0 satisfies >=0.3.0 but not >=0.4.0
	old := templateVersionSatisfied
	templateVersionSatisfied = func(minEngine string) bool { return minEngine != "0.4.0" }
	defer func() { templateVersionSatisfied = old }()

	idx := &index{Schema: 1, Templates: []catalogTemplate{
		{Name: "normal", Latest: "0.2.0", Versions: []catalogVersion{
			{Version: "0.2.0", MinEngine: "0.4.0"}, // incompatible
			{Version: "0.1.0", MinEngine: "0.3.0"}, // compatible
		}},
		{Name: "future", Latest: "1.0.0", Versions: []catalogVersion{
			{Version: "1.0.0", MinEngine: "0.3.0"}, // compatible engine but unregistered
		}},
	}}
	registered := []string{"normal"}
	hasLocal := func(n string) bool { return false }
	isCached := func(n, v string) bool { return n == "normal" && v == "0.1.0" }

	rows := buildTemplateRows(idx, registered, hasLocal, isCached)

	normalRow, ok := findRow(rows, "normal")
	if !ok || !normalRow.renderable {
		t.Fatalf("normal row missing or not renderable: %+v", normalRow)
	}
	if v, _ := findVer(normalRow.versions, "0.1.0"); !v.selectable || !v.cached {
		t.Errorf("normal 0.1.0 = %+v, want selectable and cached", v)
	}
	if v, _ := findVer(normalRow.versions, "0.2.0"); v.selectable || v.reason == "" {
		t.Errorf("normal 0.2.0 = %+v, want not selectable with a reason", v)
	}

	futureRow, ok := findRow(rows, "future")
	if !ok || futureRow.renderable || futureRow.reason != unsupportedReason {
		t.Fatalf("future row = %+v, want unsupported", futureRow)
	}
	if v, _ := findVer(futureRow.versions, "1.0.0"); v.selectable {
		t.Errorf("future 1.0.0 selectable, want disabled (unsupported build)")
	}
}

func TestBuildTemplateRowsAddsLocalRow(t *testing.T) {
	idx := catalog("normal", catalogVersion{Version: "0.1.0", MinEngine: "0.3.0"})
	rows := buildTemplateRows(idx, []string{"normal"},
		func(n string) bool { return n == "normal" },
		func(n, v string) bool { return false })

	row, ok := findRow(rows, "normal")
	if !ok {
		t.Fatal("normal row missing")
	}
	if len(row.versions) == 0 || row.versions[0].version != localVersion {
		t.Fatalf("first version = %+v, want the synthetic local row", row.versions)
	}
	if !row.versions[0].selectable {
		t.Error("local row should be selectable for a renderable template")
	}
}

// A registered template with local assets but no catalog entry still appears
func TestBuildTemplateRowsLocalOnlyTemplate(t *testing.T) {
	rows := buildTemplateRows(nil, []string{"normal"},
		func(n string) bool { return n == "normal" },
		func(n, v string) bool { return false })

	row, ok := findRow(rows, "normal")
	if !ok || len(row.versions) != 1 || row.versions[0].version != localVersion {
		t.Fatalf("local-only row = %+v, want a single local version", row)
	}
}
