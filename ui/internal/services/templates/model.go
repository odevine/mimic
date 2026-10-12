package templates

import (
	"slices"
	"sort"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/engine/version"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
)

// defaultVersionSatisfied is the production engine-compatibility check
func defaultVersionSatisfied(minEngine string) bool { return version.Satisfies(minEngine) }

// versionRow is one selectable version of a template in the manager. selectable
// is false when the engine cannot render it, and reason then says why
type versionRow struct {
	version    string
	size       int64
	cached     bool
	selectable bool
	reason     string
}

// templateRow is one template in the manager: its versions, and whether it
// renders standard cards. description is the template's own, from the catalog or
// else from the installed copy
type templateRow struct {
	name        string
	description string
	standard    bool
	// shapes are the faces the template renders, when they are known: from its
	// manifest once it is installed, and from the catalog before that when the
	// catalog says
	shapes   []template.Shape
	versions []versionRow
}

// buildTemplateRows assembles the manager's model from the catalog, the
// installed templates, and the cache. It is pure so it can be tested without any
// UI. A version is selectable when the engine satisfies its minEngine, and a
// version in a bundle format the engine does not read is not listed. A loose
// developer directory contributes a synthetic local version, and an installed
// template with no catalog entry still appears, so an offline developer sees it
func buildTemplateRows(idx *catalog.Index, installed []pipeline.Installed, hasLocal func(id string) bool, isCached func(id, ver string) bool) []templateRow {
	byID := make(map[string]pipeline.Installed, len(installed))
	for _, in := range installed {
		byID[in.ID] = in
	}

	var rows []templateRow
	seen := map[string]bool{}
	if idx != nil {
		for _, t := range idx.Templates {
			row := templateRow{name: t.Name, description: t.Description, standard: true, shapes: t.Shapes}
			if len(t.Shapes) > 0 {
				row.standard = slices.Contains(t.Shapes, pipeline.PrimaryShape)
			}
			if in, ok := byID[t.Name]; ok {
				row.description, row.standard, row.shapes = in.Description, slices.Contains(in.Shapes, pipeline.PrimaryShape), in.Shapes
			}
			if hasLocal(t.Name) {
				row.versions = append(row.versions, localRow())
			}
			for _, v := range t.Versions {
				if v.Readable() {
					row.versions = append(row.versions, catalogRow(v, isCached(t.Name, v.Version)))
				}
			}
			// A template with nothing this engine reads is not worth a row
			if len(row.versions) == 0 {
				continue
			}
			rows = append(rows, row)
			seen[t.Name] = true
		}
	}
	// Installed templates with no catalog entry
	for _, in := range installed {
		if seen[in.ID] {
			continue
		}
		row := templateRow{name: in.ID, description: in.Description, standard: slices.Contains(in.Shapes, pipeline.PrimaryShape), shapes: in.Shapes}
		if hasLocal(in.ID) {
			row.versions = append(row.versions, localRow())
		}
		cached, _ := catalog.CachedVersions(in.ID)
		for _, v := range cached {
			row.versions = append(row.versions, versionRow{version: v, cached: true, selectable: true})
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	return rows
}

// localRow is the synthetic loose-directory version
func localRow() versionRow {
	return versionRow{version: pipeline.LocalVersion, cached: true, selectable: true}
}

// catalogRow turns a catalog version into a model row, deciding selectability
func catalogRow(v catalog.Version, cached bool) versionRow {
	r := versionRow{version: v.Version, size: v.Size, cached: cached}
	switch {
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
	if version == pipeline.LocalVersion {
		return "local (developer assets)"
	}
	return version
}
