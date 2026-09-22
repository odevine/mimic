// Command mimic-ui is a desktop front end for the mimic card renderer. It
// searches Scryfall with full query syntax, previews a match through the normal
// template, and saves the result to disk
package main

import (
	"context"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/odevine/mimic/engine/card"
)

// appID namespaces this app's stored preferences, including recent searches
const appID = "dev.devine.mimic"

// netTimeout bounds a search or a render, matching the rendercard CLI
const netTimeout = 30 * time.Second

// previewMinSize is the card preview's minimum footprint, at the 5:7 aspect of
// a Magic card so the contain-fit image has room without dwarfing the results
var previewMinSize = fyne.NewSize(370, 518)

func main() {
	fyneApp := app.NewWithID(appID)
	win := fyneApp.NewWindow("mimic")
	prefs := fyneApp.Preferences()

	pipe := &renderPipeline{client: card.NewClient()}
	defer pipe.close()

	// Resolve the startup template: a persisted selection when it is available
	// without a download, otherwise the default network-free chain for normal
	at, source := startupTemplate(prefs)
	pipe.install(at)

	a := &ui{
		pipe:          pipe,
		prefs:         prefs,
		win:           win,
		recents:       newRecents(prefs.StringList(recentPrefKey)),
		activeName:    at.name,
		activeVersion: at.version,
	}

	buildUI(a)

	// Keep the default template up to date in the background so launch never
	// blocks on a large download. A loose developer directory is the intended
	// source when present, and an explicitly restored selection is the user's
	// choice, so neither is auto-updated
	if source == sourceBundle || source == sourcePlaceholder {
		go updateTemplate(a, at.name)
	}

	win.Resize(fyne.NewSize(1240, 760))
	win.ShowAndRun()
}

// startupTemplate builds the template to render at launch. It restores the
// persisted selection only when it needs no download (a loose dir or an
// already-cached bundle), so launch is never blocked, and otherwise falls back
// to the default network-free chain for normal
func startupTemplate(prefs fyne.Preferences) (*activeTemplate, assetSource) {
	name := prefs.String(templateNamePrefKey)
	ver := prefs.String(templateVersionPrefKey)
	restorable := name != "" && ((ver == localVersion && looseDir(name) != "") || isVersionCached(name, ver))
	if restorable {
		if at, err := activeFromVersion(context.Background(), name, ver, nil); err == nil {
			return at, sourceExplicit
		} else {
			log.Printf("mimic-ui: restoring template %s %s failed: %v", name, ver, err)
		}
	}
	at, source, err := resolveActiveTemplate("normal")
	if err != nil {
		log.Fatalf("mimic-ui: resolving template: %v", err)
	}
	return at, source
}

// updateTemplate downloads the latest compatible version of a template and swaps
// it in. It runs in the background at startup, so any failure is logged and the
// app keeps rendering from whatever it resolved to. The swap and re-render run
// on the UI goroutine
func updateTemplate(a *ui, name string) {
	at, err := autoLatestActive(context.Background(), name)
	if err != nil {
		log.Printf("mimic-ui: template update skipped: %v", err)
		return
	}
	fyne.Do(func() {
		a.setActiveTemplate(at)
		a.rerenderCurrent()
	})
}

// buildUI constructs the widgets, wires their callbacks, and lays out the
// window. Everything here runs on the Fyne goroutine
func buildUI(a *ui) {
	a.entry = widget.NewSelectEntry(a.recents.list())
	a.entry.SetPlaceHolder("Search Scryfall, e.g. t:goblin c:r cmc=1")
	a.entry.OnSubmitted = func(q string) { a.runSearch(q) }

	a.searchBtn = widget.NewButton("Search", func() { a.runSearch(a.entry.Text) })
	searchBar := container.NewBorder(nil, nil, nil, a.searchBtn, a.entry)

	// Template controls: the active-template indicator and the manager button
	a.templatesBtn = widget.NewButton("Templates…", func() { openTemplateManager(a) })
	a.templateLabel = widget.NewLabel("")
	a.updateTemplateIndicator()
	templateBar := container.NewBorder(nil, nil, a.templatesBtn, nil, a.templateLabel)

	a.resultsList = widget.NewList(
		func() int { return len(a.results) },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id < 0 || id >= len(a.results) {
				return
			}
			o.(*widget.Label).SetText(rowText(a.results[id]))
		},
	)
	a.resultsList.OnSelected = func(id widget.ListItemID) { a.selectResult(id) }

	a.editor = newCardEditor(func() { a.renderEdited() })
	a.renderBtn = widget.NewButton("Render preview", func() { a.renderEdited() })
	a.renderBtn.Importance = widget.HighImportance
	a.renderBtn.Disable()
	a.resetBtn = widget.NewButton("Reset to card", func() { a.resetEdits() })
	a.resetBtn.Disable()
	editorButtons := container.NewBorder(nil, nil, nil,
		container.NewHBox(a.resetBtn, a.renderBtn))
	editorPane := container.NewBorder(nil, editorButtons, nil, nil,
		container.NewVScroll(a.editor.content))

	a.preview = canvas.NewImageFromImage(nil)
	a.preview.FillMode = canvas.ImageFillContain
	a.preview.SetMinSize(previewMinSize)

	a.saveBtn = widget.NewButton("Save PNG…", func() { a.save() })
	a.saveBtn.Disable()

	a.status = widget.NewLabel("Search for a card to begin.")
	bottomBar := container.NewBorder(nil, nil, nil, a.saveBtn, a.status)

	// Three panes: results pick a card, the editor tweaks its fields, the
	// preview shows the render
	inner := container.NewHSplit(editorPane, container.NewPadded(a.preview))
	inner.SetOffset(0.46)
	outer := container.NewHSplit(a.resultsList, inner)
	outer.SetOffset(0.24)

	top := container.NewVBox(searchBar, templateBar)
	content := container.NewBorder(top, bottomBar, nil, nil, outer)
	a.win.SetContent(content)
	a.win.Canvas().Focus(a.entry)
}
