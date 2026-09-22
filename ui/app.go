package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/odevine/mimic/engine/card"
)

// recentPrefKey is where the recent-search list persists between launches.
// templateNamePrefKey and templateVersionPrefKey remember the chosen template so
// it is restored on the next launch
const (
	recentPrefKey          = "recent.searches"
	templateNamePrefKey    = "template.name"
	templateVersionPrefKey = "template.version"
)

// app holds the widgets and the mutable state a search or render touches. Every
// field is read and written only on the Fyne UI goroutine: the background
// search and render goroutines touch nothing here, they hand their results back
// through fyne.Do, which runs the callback on the UI goroutine. That confinement
// is what keeps the state consistent, so no field needs a lock
type ui struct {
	pipe  *renderPipeline
	prefs fyne.Preferences
	win   fyne.Window

	entry         *widget.SelectEntry
	searchBtn     *widget.Button
	resultsList   *widget.List
	editor        *cardEditor
	renderBtn     *widget.Button
	resetBtn      *widget.Button
	preview       *canvas.Image
	status        *widget.Label
	saveBtn        *widget.Button
	templatesBtn  *widget.Button
	templateLabel *widget.Label

	// activeName and activeVersion mirror the pipeline's active template for the
	// indicator and the manager's active tag. UI-goroutine only, like the rest
	activeName    string
	activeVersion string

	recents *recents
	results []*card.Data // read only on the UI goroutine

	// origData is the card as fetched, kept so the editor can reset to it. art
	// is that card's art crop, cached so an edit re-renders without refetching.
	// artMissing records that the fetched card had art the download could not
	// get, so a re-render still reports it. All three are UI-goroutine only
	origData   *card.Data
	art        image.Image
	artMissing bool

	// gen is bumped by every search, selection, and edit render. A goroutine
	// captures it at the start and drops its result if gen has moved on, so a
	// slow render whose selection has since been replaced by a new search never
	// repaints the panes. cancel aborts the previous operation, whatever it was
	gen          uint64
	cancel       context.CancelFunc
	rendered     image.Image
	renderedName string
}

// runSearch queries Scryfall in the background and applies the results only if
// no newer search has started since. Starting a search cancels the previous one
func (a *ui) runSearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}

	ctx, cancel, seq := a.beginOp()
	a.searchBtn.Disable()
	a.status.SetText(fmt.Sprintf("Searching for %q…", query))

	go func() {
		defer cancel()
		results, err := a.pipe.client.Search(ctx, query)
		fyne.Do(func() {
			if a.staleOp(seq) {
				return
			}
			a.searchBtn.Enable()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				a.status.SetText("Search failed: " + err.Error())
				return
			}
			a.rememberQuery(query)
			a.applyResults(query, results)
		})
	}()
}

// applyResults swaps in the new results, refreshes the list, and reports the
// count. It runs on the UI goroutine
func (a *ui) applyResults(query string, results []*card.Data) {
	a.results = results
	a.resultsList.UnselectAll()
	a.resultsList.Refresh()
	a.resultsList.ScrollToTop()
	switch len(results) {
	case 0:
		a.status.SetText(fmt.Sprintf("No cards match %q", query))
	case 1:
		a.status.SetText("1 result")
	default:
		a.status.SetText(fmt.Sprintf("%d results", len(results)))
	}
}

// selectResult fetches the chosen card's art, loads it into the editor, and
// renders it, all in the background and applied only if no newer selection or
// search has superseded it. The art is cached so later edits skip the network
func (a *ui) selectResult(id widget.ListItemID) {
	if id < 0 || id >= len(a.results) {
		return
	}
	d := a.results[id]

	ctx, cancel, seq := a.beginOp()
	a.saveBtn.Disable()
	a.status.SetText("Loading " + d.Name + "…")

	go func() {
		defer cancel()
		art, artErr := a.pipe.fetchArt(ctx, d)
		img, err := a.pipe.render(ctx, d, art)
		fyne.Do(func() {
			if a.staleOp(seq) {
				return
			}
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.status.SetText("Render failed: " + err.Error())
				}
				return
			}
			a.origData = d
			a.art = art
			a.artMissing = artErr != nil
			a.editor.load(d)
			a.renderBtn.Enable()
			a.resetBtn.Enable()
			a.showRendered(d, img)
		})
	}()
}

// renderEdited re-renders the card with the editor's current field values,
// reusing the cached art. It supersedes any in-flight render
func (a *ui) renderEdited() {
	if a.origData == nil {
		return
	}
	edited := a.editor.apply(a.origData)

	ctx, cancel, seq := a.beginOp()
	art := a.art
	a.saveBtn.Disable()
	a.status.SetText("Rendering " + edited.Name + "…")

	go func() {
		defer cancel()
		img, err := a.pipe.render(ctx, edited, art)
		fyne.Do(func() {
			if a.staleOp(seq) {
				return
			}
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.status.SetText("Render failed: " + err.Error())
				}
				return
			}
			a.showRendered(edited, img)
		})
	}()
}

// resetEdits restores the editor to the card as fetched and re-renders it
func (a *ui) resetEdits() {
	if a.origData == nil {
		return
	}
	a.editor.load(a.origData)
	a.renderEdited()
}

// beginOp starts a new user operation: it bumps the generation, cancels
// whatever operation was in flight, and returns the new context, its cancel for
// the caller to defer, and its generation. A search, a selection, and an edit
// render all go through it, so any one supersedes the others. It runs on the UI
// goroutine
func (a *ui) beginOp() (context.Context, context.CancelFunc, uint64) {
	a.gen++
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
	a.cancel = cancel
	return ctx, cancel, a.gen
}

// showRendered puts the image in the preview and enables saving. It runs on the
// UI goroutine
func (a *ui) showRendered(d *card.Data, img image.Image) {
	a.rendered = img
	a.renderedName = d.Name

	a.preview.Image = img
	a.preview.Refresh()
	a.saveBtn.Enable()

	if a.artMissing {
		a.status.SetText("Rendered " + d.Name + " (art unavailable)")
		return
	}
	a.status.SetText("Rendered " + d.Name)
}

// setActiveTemplate installs a new template, records it for the indicator, and
// persists the choice so the next launch restores it. It runs on the UI
// goroutine
func (a *ui) setActiveTemplate(at *activeTemplate) {
	a.pipe.install(at)
	a.activeName = at.name
	a.activeVersion = at.version
	a.updateTemplateIndicator()
	if a.prefs != nil {
		a.prefs.SetString(templateNamePrefKey, at.name)
		a.prefs.SetString(templateVersionPrefKey, at.version)
	}
}

// updateTemplateIndicator refreshes the top-bar label with the active template.
// It runs on the UI goroutine
func (a *ui) updateTemplateIndicator() {
	if a.templateLabel == nil {
		return
	}
	a.templateLabel.SetText("Template: " + templateDisplay(a.activeName, a.activeVersion))
}

// rerenderCurrent re-renders the loaded card through the active template, so a
// template switch updates the preview without refetching art. A no-op when no
// card is loaded. It runs on the UI goroutine
func (a *ui) rerenderCurrent() {
	if a.origData == nil {
		return
	}
	a.renderEdited()
}

// templateDisplay names a template and version for the indicator. An empty
// version is the placeholder fallback
func templateDisplay(name, version string) string {
	switch version {
	case "":
		return name + " (placeholder)"
	case localVersion:
		return name + " · local"
	default:
		return name + " · " + version
	}
}

// save writes the current preview to a PNG the user picks
func (a *ui) save() {
	img := a.rendered
	name := a.renderedName
	if img == nil {
		return
	}

	fd := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, a.win)
			return
		}
		if w == nil {
			return // cancelled
		}
		defer w.Close()
		if err := png.Encode(w, img); err != nil {
			dialog.ShowError(err, a.win)
			return
		}
		a.status.SetText("Saved " + w.URI().Name())
	}, a.win)
	fd.SetFileName(pngFilename(name))
	fd.Show()
}

// rememberQuery records the query and refreshes the search box's suggestions
// and the stored list. It runs on the UI goroutine
func (a *ui) rememberQuery(query string) {
	a.recents.add(query)
	a.entry.SetOptions(a.recents.list())
	if a.prefs != nil {
		a.prefs.SetStringList(recentPrefKey, a.recents.list())
	}
}

// staleOp reports whether a newer operation has started since gen was captured,
// so a goroutine can drop a result the user has already moved past
func (a *ui) staleOp(gen uint64) bool {
	return gen != a.gen
}

// rowText is one result-list line: the card name, its set, and its type
func rowText(d *card.Data) string {
	parts := []string{d.Name}
	if d.SetCode != "" {
		parts = append(parts, strings.ToUpper(d.SetCode))
	}
	if d.TypeLine != "" {
		parts = append(parts, d.TypeLine)
	}
	return strings.Join(parts, "  ·  ")
}

// pngFilename turns a card name into a safe .png filename
func pngFilename(name string) string {
	if name == "" {
		return "card.png"
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, name)
	return cleaned + ".png"
}
