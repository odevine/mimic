package main

import (
	"sort"

	"github.com/odevine/mimic/engine/version"
)

// defaultVersionSatisfied is the production engine-compatibility check
func defaultVersionSatisfied(minEngine string) bool { return version.Satisfies(minEngine) }

// versionRow is one selectable version of a template in the manager. selectable
// is false when the engine cannot render it, and reason then says why
type versionRow struct {
	version    string
	cached     bool
	selectable bool
	reason     string
}

// templateRow is one template in the manager: its versions and whether this
// build can render it at all. A template the engine has no code for is listed
// but not renderable, so the catalog stays honest about what exists
type templateRow struct {
	name       string
	renderable bool
	reason     string
	versions   []versionRow
}

// unsupportedReason is shown when the running build has no code for a template
const unsupportedReason = "not supported by this build"

// buildTemplateRows assembles the manager's model from the catalog, the engine's
// registered template names, and the cache. It is pure so it can be tested
// without any UI. A template is renderable when it is registered; a version is
// selectable when its template is renderable and the engine satisfies its
// minEngine. A loose developer directory contributes a synthetic local version,
// and a registered template with local assets but no catalog entry still
// appears, so an offline developer sees it
func buildTemplateRows(idx *index, registered []string, hasLocal func(name string) bool, isCached func(name, ver string) bool) []templateRow {
	regSet := make(map[string]bool, len(registered))
	for _, n := range registered {
		regSet[n] = true
	}

	var rows []templateRow
	seen := map[string]bool{}
	if idx != nil {
		for _, t := range idx.Templates {
			row := templateRow{name: t.Name, renderable: regSet[t.Name]}
			if !row.renderable {
				row.reason = unsupportedReason
			}
			if hasLocal(t.Name) {
				row.versions = append(row.versions, localRow(row.renderable))
			}
			for _, v := range t.Versions {
				row.versions = append(row.versions, catalogRow(v, row.renderable, isCached(t.Name, v.Version)))
			}
			rows = append(rows, row)
			seen[t.Name] = true
		}
	}
	// Registered templates with local assets but no catalog entry
	for _, n := range registered {
		if seen[n] || !hasLocal(n) {
			continue
		}
		rows = append(rows, templateRow{name: n, renderable: true, versions: []versionRow{localRow(true)}})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	return rows
}

// localRow is the synthetic loose-directory version
func localRow(renderable bool) versionRow {
	r := versionRow{version: localVersion, cached: true, selectable: renderable}
	if !renderable {
		r.reason = unsupportedReason
	}
	return r
}

// catalogRow turns a catalog version into a model row, deciding selectability
func catalogRow(v catalogVersion, renderable, cached bool) versionRow {
	r := versionRow{version: v.Version, cached: cached}
	switch {
	case !renderable:
		r.reason = unsupportedReason
	case !templateVersionSatisfied(v.MinEngine):
		r.reason = "needs engine " + v.MinEngine
	default:
		r.selectable = true
	}
	return r
}

// templateVersionSatisfied is a seam over version.Satisfies so a test can drive
// the engine-compatibility decision without stamping a binary
var templateVersionSatisfied = defaultVersionSatisfied

// versionLabel renders a version for display, naming the synthetic local one
func versionLabel(version string) string {
	if version == localVersion {
		return "local (developer assets)"
	}
	return version
}
