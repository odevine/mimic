// Command mimic-ui is a desktop front end for the mimic card renderer. It
// searches Scryfall with full query syntax, previews a match through the normal
// template, and saves the result to disk
package main

import (
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/engine/template/normal"
)

// appID namespaces this app's stored preferences, including recent searches
const appID = "dev.devine.mimic"

// netTimeout bounds a search or a render, matching the rendercard CLI
const netTimeout = 30 * time.Second

// previewMinSize is the card preview's minimum footprint, at the 5:7 aspect of
// a Magic card so the contain-fit image has room without dwarfing the results
var previewMinSize = fyne.NewSize(370, 518)

func main() {
	assetDir, cleanup, err := resolveAssetDir()
	if err != nil {
		log.Fatalf("mimic-ui: resolving assets: %v", err)
	}
	defer cleanup()

	tmpl, err := template.Get("normal")
	if err != nil {
		log.Fatalf("mimic-ui: loading template: %v", err)
	}
	if fontDir := resolveFontDir(); fontDir != "" {
		if nt, ok := tmpl.(*normal.Template); ok {
			nt.FontDir = fontDir
		}
	}

	fyneApp := app.NewWithID(appID)
	win := fyneApp.NewWindow("mimic")

	prefs := fyneApp.Preferences()
	a := &ui{
		pipe: &renderPipeline{
			client: card.NewClient(),
			tmpl:   tmpl,
			assets: template.NewFSAssetProvider(assetDir),
		},
		prefs:   prefs,
		win:     win,
		recents: newRecents(prefs.StringList(recentPrefKey)),
	}

	buildUI(a)
	win.Resize(fyne.NewSize(1240, 760))
	win.ShowAndRun()
}

// buildUI constructs the widgets, wires their callbacks, and lays out the
// window. Everything here runs on the Fyne goroutine
func buildUI(a *ui) {
	a.entry = widget.NewSelectEntry(a.recents.list())
	a.entry.SetPlaceHolder("Search Scryfall, e.g. t:goblin c:r cmc=1")
	a.entry.OnSubmitted = func(q string) { a.runSearch(q) }

	a.searchBtn = widget.NewButton("Search", func() { a.runSearch(a.entry.Text) })
	searchBar := container.NewBorder(nil, nil, nil, a.searchBtn, a.entry)

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

	content := container.NewBorder(searchBar, bottomBar, nil, nil, outer)
	a.win.SetContent(content)
	a.win.Canvas().Focus(a.entry)
}
