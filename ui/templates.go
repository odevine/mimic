package main

import (
	"context"
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/odevine/mimic/engine/template"
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

// templateManager is the Templates dialog: it lists the model rows and drives a
// switch, including a download with a progress bar
type templateManager struct {
	a        *ui
	dialog   dialog.Dialog
	list     *fyne.Container
	progress *widget.ProgressBar
	status   *widget.Label
}

// openTemplateManager opens the Templates dialog. It fetches the catalog in the
// background, falling back to the cached copy offline, then fills the list on
// the UI goroutine
func openTemplateManager(a *ui) {
	m := &templateManager{
		a:        a,
		list:     container.NewVBox(),
		progress: widget.NewProgressBar(),
		status:   widget.NewLabel(""),
	}
	m.progress.Hide()

	body := container.NewBorder(nil, container.NewVBox(m.progress, m.status), nil, nil,
		container.NewVScroll(m.list))
	m.dialog = dialog.NewCustom("Templates", "Close", body, a.win)
	m.dialog.Resize(fyne.NewSize(520, 560))

	m.list.Add(widget.NewLabel("Loading catalog…"))
	m.dialog.Show()

	go func() {
		idx, err := fetchIndex(context.Background())
		fyne.Do(func() {
			if err != nil {
				// No live catalog and no cache: still show local/registered rows
				idx = nil
			}
			m.populate(idx)
		})
	}()
}

// populate rebuilds the list from the current model. It runs on the UI goroutine
func (m *templateManager) populate(idx *index) {
	m.list.RemoveAll()
	rows := buildTemplateRows(idx, template.Names(), func(n string) bool { return looseDir(n) != "" }, isVersionCached)
	if len(rows) == 0 {
		m.list.Add(widget.NewLabel("No catalog available. Connect to fetch templates."))
		m.list.Refresh()
		return
	}
	for _, row := range rows {
		m.list.Add(m.templateSection(row))
	}
	m.list.Refresh()
}

// templateSection is one template's block: a heading and its version rows
func (m *templateManager) templateSection(row templateRow) fyne.CanvasObject {
	heading := widget.NewLabelWithStyle(row.name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	items := []fyne.CanvasObject{heading}
	if row.reason != "" {
		items = append(items, widget.NewLabel("  "+row.reason))
	}
	for _, v := range row.versions {
		items = append(items, m.versionRow(row.name, v))
	}
	items = append(items, widget.NewSeparator())
	return container.NewVBox(items...)
}

// versionRow is one version line: its label, a tag, and an action
func (m *templateManager) versionRow(name string, v versionRow) fyne.CanvasObject {
	label := widget.NewLabel("   " + versionLabel(v.version))

	active := name == m.a.activeName && v.version == m.a.activeVersion
	var trailing fyne.CanvasObject
	switch {
	case active:
		trailing = widget.NewLabelWithStyle("active", fyne.TextAlignTrailing, fyne.TextStyle{Italic: true})
	case !v.selectable:
		trailing = widget.NewLabel(v.reason)
	case v.cached:
		trailing = widget.NewButton("Select", func() { m.selectVersion(name, v.version) })
	default:
		btn := widget.NewButton("Download & select", func() { m.selectVersion(name, v.version) })
		btn.Importance = widget.HighImportance
		trailing = btn
	}
	return container.NewBorder(nil, nil, label, trailing)
}

// selectVersion switches the app to (name, version), downloading first when the
// version is not cached. It runs the work in the background and applies the
// result on the UI goroutine
func (m *templateManager) selectVersion(name, version string) {
	m.setBusy(fmt.Sprintf("Preparing %s %s…", name, versionLabel(version)))

	var progress func(done, total int64)
	if !isVersionCached(name, version) && version != localVersion {
		last := -1.0
		progress = func(done, total int64) {
			if total <= 0 {
				return
			}
			frac := float64(done) / float64(total)
			if frac-last < 0.01 && frac < 1 {
				return
			}
			last = frac
			fyne.Do(func() { m.progress.SetValue(frac) })
		}
	}

	go func() {
		at, err := activeFromVersion(context.Background(), name, version, progress)
		fyne.Do(func() {
			if err != nil {
				m.setError(err)
				return
			}
			m.a.setActiveTemplate(at)
			m.dialog.Hide()
			m.a.rerenderCurrent()
		})
	}()
}

// setBusy shows the progress bar and a status line during a switch
func (m *templateManager) setBusy(text string) {
	m.progress.SetValue(0)
	m.progress.Show()
	m.status.SetText(text)
}

// setError reports a failed switch in the dialog and hides the progress bar
func (m *templateManager) setError(err error) {
	m.progress.Hide()
	m.status.SetText("Failed: " + err.Error())
}

// versionLabel renders a version for display, naming the synthetic local one
func versionLabel(version string) string {
	if version == localVersion {
		return "local (developer assets)"
	}
	return version
}
