# mimic ui

A desktop app for the mimic card renderer. It opens a window that shows an
embedded web frontend through the system webview, searches Scryfall, previews a
card through the active template, and saves the result. The frontend and the Go
services talk through the window, so nothing listens on the network.

The frontend is embedded with `//go:embed`, so there is no Node or npm step. The
window needs the system webview, which needs cgo on macOS and Linux. Only
`internal/desktop` imports it, and every other package builds and tests with
`CGO_ENABLED=0`.

## System requirements

| System  | Needs                                                                                     |
| ------- | ----------------------------------------------------------------------------------------- |
| macOS   | macOS 12 or later, Apple silicon or Intel                                                 |
| Windows | Windows 10 or 11, 64-bit Intel or Arm. The installer adds the WebView2 runtime if needed |
| Linux   | GTK 3 and WebKitGTK 4.1, as Ubuntu 22.04 and later package them                          |

A render holds a few document-sized buffers, so memory decides how many cards
render at once. The app plans for the window's own memory, about half a gigabyte,
before it sizes a run, and a computer with 8 GB typically renders full-size cards one
or two at a time.

## Running it

```
go run .            # macOS and Windows
go run -tags gtk3 . # Linux
```

Run it from a checkout of the full repository, not the `ui` directory in
isolation, so the Go workspace resolves the local `engine` module. Linux needs a
C compiler and the `libgtk-3-dev` and `libwebkit2gtk-4.1-dev` packages. Wails v3
builds against GTK 4 unless told otherwise, and the `gtk3` tag selects the GTK 3
and WebKitGTK 4.1 that Ubuntu 22.04 ships.

Flags:

- `-fonts <dir>` offers the fonts in dir, subfolders included, to assign to roles,
  ahead of every other font source.
- `-smoke` runs the launch check against a scratch config folder and exits. It
  opens the window, boots the page, renders a card through the bridge, loads the
  images, drives a few flows in the page (a popover, the settings dialog, a
  rendered list and the Single preview) and exits zero when all of that works.

A build made with `-tags dev` turns on the webview's developer tools.

## Using it

The page is one shell with a mode rail down the left: Single, From a List, and
From Your Art make cards, Templates holds everything about the frames cards
render with, Run keeps the record of a batch, and Editor is where a template of
your own is built. The active template's name
sits in the top bar and opens Templates, and settings and a keyboard reference
sit beside it.

Templates has three tabs. Library lists the catalog and any template installed
from a local folder, downloads a version, and selects the active template for
standard cards. A template that renders only other faces, such as transform, is
installed there rather than selected. Defaults picks which installed template
renders each kind of face, and a change there applies at once. Overrides holds
the per-template and global overrides.

In Single, the left column searches Scryfall with its
full query syntax (for example `t:goblin c:r cmc=1`) and lists one row per card
name. Selecting a match fills the editor, where a printing dropdown switches
between every printing of the card. Fields you have not edited follow the new
printing, and edited ones keep your value. Each edited field carries a dot and
a revert control. The mana cost and rules text show their braced codes as the
engine's own pips as you type, with a code the engine does not draw flagged,
and a palette inserts symbols for anyone who does not know the syntax. Any text
field can be redacted by wrapping words in `~~`, as in
`You ~~can't lose the game~~.`, which draws a marker bar over the words in place
of them. A span left open runs to the end of its paragraph.

Render draws the current values into the preview, which zooms to fit, 100% and
200% and pans by dragging. Save asks where to put the card, then writes it at the output resolution.
`⌘↵` renders, `⌘S` saves, `/` jumps to search, and `?` lists the rest.

A card's art is fetched once when you select it and reused across edits, so
tweaking a field re-renders without another download. With no local template
assets present the app falls back to placeholder frame layers, the same as the
`rendercard` command in the engine module.

From a List takes a pasted list, a dropped file, or one opened from disk. It
reads plain names, a quantity prefix such as `4x Lightning Bolt`, Arena exports
whose `(2X2) 117` suffix picks that printing, CSV with a header row, and lines
starting with `?` as Scryfall queries. Section headers such as Sideboard group
the rows below them. The format is detected, and a dropdown overrides it when
the guess is wrong.

Resolve looks every line up on Scryfall and fills the review table as results
arrive. A name with no printing given takes Scryfall's default printing. A name
Scryfall does not know exactly lands as ambiguous with its likely matches to
pick from, and one with no match at all offers a search. The table opens on the
rows that need attention when there are any. Unticking a row leaves it out of
the render, and that survives resolving the list again.

In a CSV, a `name` column is looked up and any other column named after an
editor field, such as `power` or `oracle`, overrides that field on the matched
card. A row with a truthy `custom` column skips the lookup and renders from its
own fields, and so does a row whose name matches nothing when it has a `type`.

Render writes one image per card into the output folder, chosen with the system folder
dialog from the footer and remembered with a short list of recent folders. A
file is named after the card and the printing it came from, as in
`Sol Ring [C21-263].jpg`, and rendering into the same folder again overwrites
it. Cards are JPEG at quality 95 by default, converted straight from the
engine's render to the planes a JPEG stores, and the Image format setting
switches them to PNG, whose compression setting then applies. A second copy of the same printing in one run gets a `(2)` suffix, and a
quantity is recorded in the run report rather than written as extra files.
A run renders as many cards at once as the computer's memory and processors hold, and the Concurrency setting picks a fixed number instead.

A double-faced card renders one image per face, each named after that face, as in
`Delver of Secrets [MID-51].jpg` and `Insectile Aberration [MID-51].jpg`, and
the run treats each face as its own card. Each face renders through the
template chosen for its shape: standard cards through the active template, and
every other shape through the one picked under Templates, Defaults, or the
first installed template that supports it. A face no installed template
supports is marked unsupported while the other face still renders. The
single-card editor previews either face with Back face, and its fields edit the
front. A split or fuse card has no front to edit, since it prints two halves, so
the editor shows each half's name, cost, colors, type, rules, and flavor in its
own section. The same fields are the `half1` and `half2` columns of a CSV list,
such as `half2Oracle`.

The format beside the folder switches Render to an
[MPC Autofill](https://github.com/chilli-axe/mpc-autofill) project instead,
laid out by the engine's `mpcfill` package, whose files are always PNG: fronts in `fronts/`, the backs of
double-faced cards in `backs/`, the shared cardback in `cardback/`, and a
`cards.xml` order file beside them. Its options set the cardstock, foil, and
the cardback, an uploaded PNG or JPEG kept beside `prefs.json` and scaled down
to the 1500 DPI the website shows. A project holds
at most 612 cards, counting copies, and needs every face of a double-faced
card, so the footer flags a list too big or with a face no installed template
renders. `cards.xml` is written only once every card has rendered in this
project, never on the strength of a file an earlier render left in the folder,
and Render asks first when the folder already holds a project's files.
After failures, Retry failed finishes the project, and after a Stop the list
renders again. MPC Autofill's website then takes the folder as a local folder,
followed by an import of `cards.xml`, and its desktop tool takes the folder
with `-d`.

Selecting a row and pressing `e`, or choosing the sliders button on it, opens the
row inspector beside the table. It is the same field editor Single uses, and it
edits that one row: each field you change is kept on the row as a field override,
the way a CSV column is, and a render draws the row with them. A field you have
not changed stays off the row. Open in Single previews the row's card with its
edits on it, and Apply to row brings what you changed there back. Revert row
removes every edit. What you change on a row, its fields and its quantity, is
kept when you resolve the same list again, for lines whose text has not changed.

The Actions menu in the review summary works on the checked rows. It can set one
field or one quantity on all of them, remove them, and tick or untick rows: the
rows shown by the current filter, the rows that need attention, or a section. A
removal can be undone from the same menu until the next resolve.

Run shows the batch as it goes: overall progress with an estimate, a line per
card with its time or its own progress, and the run's time. The first wave of
cards, as many as render at once, load the frame layers and fonts, so that time
is shown on its own as the warm-up and the cards per second counts only what is
rendered after it. Picking a card opens its finished render beside the list and
it stays until another is picked or it is closed, so a running batch spends no
time drawing previews nobody asked for. The Log button beside Open folder opens
the run's log above the list, with Copy and Save. A failed card opens to its
error and the stage it failed at.
Stop skips every card still queued, and a card already rendering finishes
first. When the run ends, a `mimic-run-<timestamp>.json` report beside the images
records each card's outcome, printing, file, and field overrides, along with
the template, resolution, and warm-up used. Retry failed starts a new run from only the
failed cards. The app keeps the latest run, so reloading the page picks it
back up, and the template cannot be switched while a run is going.

## Development

The checks CI runs are the ones to run before sending a change:

```
go vet ./...
go test -race ./...
node --test frontend/test/*.test.mjs
go run . -smoke
```

`go test` covers the Go packages without a window, and the Node tests cover the
frontend logic that does not need a document, such as the job watcher and the
tooltip. The smoke check opens the window, so it needs a display. Add `-tags gtk3`
to the Go commands on Linux.

`cmd/throughput` renders a list through the same run service the window uses and
prints cards per second, which is the number to compare when the run loop or the
engine changes. It takes a JSON file of run rows, the shape the page sends, and
renders the list more than once, since the first pass fetches art.

```
go run ./cmd/throughput -rows rows.json -runs 3
```

`RELEASING.md` lists what to check by hand on each system before a release.

## Packaging

The Wails CLI is a tool dependency of the module, so there is nothing to install
beyond what a build needs:

```
go tool wails3 task build                  # the binary, into bin/
go tool wails3 task package VERSION=1.0.0  # what a release ships, into bin/
```

Package stamps the version into `build/darwin/Info.plist`,
`build/windows/info.json` and `build/config.yml`, so run it on a throwaway
checkout, or revert `build/` afterward. It also wants `GOWORK=off`, so the
engine version it stamps is the release the module pins. Each system packages
only itself, because the webview needs the system's own toolchain. macOS makes a
universal `.dmg` and `.zip`. Windows needs
[NSIS](https://nsis.sourceforge.io) for `makensis`, takes `ARCH=amd64` or
`ARCH=arm64` (releases ship amd64 only, which a Windows Arm PC runs under
emulation), and makes a per-user installer and a portable `.zip`. Linux makes
a Flatpak bundle and a `.deb`. The Flatpak
step needs `flatpak` and `flatpak-builder`, and installs the GNOME runtime and
SDK from Flathub when it runs.

`.github/workflows/package-ui.yml` runs all three on native runners. A release
calls it to publish, and a manual run, the weekly run and any pull request that
touches `ui/build` call it without publishing, which leaves the files as
workflow artifacts.

`cmd/pkgtool` holds the chores the tasks share across systems: stamping the
version, zipping an app for the updater, listing checksums and signing. To sign
updates, make a key pair with `go run ./cmd/pkgtool keygen`. Store the private
half as the `MIMIC_UPDATE_PRIVATE_KEY` secret and the public half as the
`MIMIC_UPDATE_PUBLIC_KEY` variable, and releases then carry a `.sig` beside each
update file, checked by the key stamped into the app. A signature is Ed25519
over the file's SHA-256, which is what the updater verifies.

## The window

The window opens where it was left. Its position and size are saved to
`prefs.json` under `window`, and a saved place that is off every connected
display, such as after unplugging a monitor, is dropped and the window opens
centered at the default size. Closing the window quits the app, and with a run
going a dialog asks first and stops the run if you agree. Starting mimic a
second time brings the first window forward instead of opening another.

The menu bar follows each platform. File holds Save Card, Open List and Choose
Output Folder, View switches modes, and Help links to this repository and shows
the versions in About. On macOS the menu owns `⌘S` and `⌘1` to `⌘6`. On Windows
and Linux the page handles those keys and the menu shows their text.

When a run finishes while the window is not in front, the app posts a system
notification with the number of cards rendered and failed, and no card names.
The Notify setting turns it off. Notifications need a packaged app, so a build
from `go run` logs that they are off.

## Updates

At launch, at most once a day, the app asks GitHub for the newest `ui/` release
and shows a strip in the top bar when it is newer and has a file for this
platform. Settings, under Updates, shows the same status with the release notes
and a Check for updates button, and a setting turns the launch check off.
Nothing downloads until you choose Install. The download is checked against the
release's `SHA256SUMS`, and against its `.sig` file when the release is signed
and the build was stamped with the public key. Restart then swaps the new
version in. An install the app cannot replace itself in, such as a system
package, offers the release page instead. A development build does not check.

## Overrides

Templates, Overrides, Global holds rules that apply to every card you render,
whether one at a time in Single or in a list. A rule has conditions on a card's
fields and actions that set or clear fields. A condition compares a field with a
value: contains, does not contain, is, is not, starts with, ends with, matches a
pattern, is empty or is not empty. Text comparisons ignore case, and a pattern is
a regular expression. All of a rule's conditions must hold, and a rule with none
applies to every card. Actions set a field to a value or clear it.

Rules apply in order from the top, and a later rule sees what an earlier one
changed. A field you edit yourself, on a card in Single or on a list row, keeps
your value, and a rule that tests that field reads it. Each rule can be turned
off, Disable all turns every rule off, and each rule shows how many rows of the
current list it holds for. In Single, a field a rule set shows its own dot until
you change it. The rules save a moment after each change, and the app checks the
whole list, so a rule that does not check, such as a pattern that does not parse,
says why and nothing is saved until it does. A run records the fields a rule set
in its report, beside the ones the list set.

The Preset menu saves the rules together with the render settings, which are the
two resolutions, the image format and PNG compression, and the MPC Autofill stock
and foil choice. A preset has a name and a version, applying one changes only
those settings, and the menu marks the one that matches what is in force now.
Rules and presets live in `prefs.json`.

Overrides for a single template's layers and text boxes need engine support and
are tracked in the issue named on the Template tab.

## The editor

The Editor builds a template from the files an author keeps, a folder with
`template.json`, a file for each layout and the images and fonts they use. Each
template there is a draft under `drafts` in the user config folder, and the app
renders it as it is on disk, so changing a draft cannot change an installed
template.

New template, in Editor or in the Templates library, starts from a plain card
with the standard text boxes at a resolution of 72 to 1200 dpi, 800 by default,
and draws it at once. Edit a copy, beside each installed version in the library,
starts from that template instead. The copy keeps the original's license and
credits and adds a credit that names the original and its version, and it is
independent of the original from then on.

The left column picks the draft, with Undo and Redo beside its name since they take
back a change made anywhere in the template, has the template's About and its canvas under
it, and picks the card the draft is drawn with, from six presets, which are real cards that are looked
up when one is chosen, Mirran Spy as a monocolored creature, Consuming Aberration for two
colors, Eerie Ultimatum for three, Norin the Wary as a legendary creature, Sensei's
Divining Top as an artifact and Flooded Strand as a land, or from a Scryfall search in its
own section, Search Scryfall, which is above the test card. A preset that cannot be looked up, with no
network, says so and leaves the card as it was. Search Scryfall and Test card are sections
like the right column's, each collapsing to its heading and given the height its reader likes
by dragging the line between the two when both are open, and kept the same way. Only the test
card starts open. About, which holds the template's name, id, layouts, details, credits, fonts
and text roles, and Canvas are each a section that opens and collapses with its arrow, both
collapsed to begin with, and whether each is open is kept. Under the presets, Properties is a part of the test card that is open to begin
with and collapses like the printing details inside it, and holds the same fields Single
edits a card with: name, cost, colors, type, rules, flavor, power,
toughness, loyalty, artist and the printing details, and the halves of a split card. A
change draws the card again a moment after the typing stops, and the card is kept as it
was changed, so a template can be tested against a rule without finding a real card that
meets it. A preset stops being highlighted once a field is changed, comes back when the
fields are put back with Revert all, and a card that is chosen next starts the fields
over. A card, a preset or one found by searching, brings its artwork, which fills the art
window, and a change to its fields keeps the art. The card is kept with the draft. The right column has three sections, Layers,
Selection and Problems, and the buttons that use or export the draft and
open its JSON stay at the bottom of the column whatever the sections do. A section's
heading is a button that collapses it down to the heading and opens it again. A
collapsed section at the bottom of the column rests against the buttons, and the
open sections share the room that is left. The line between two open sections is
dragged, or moved with the up and down arrows, to give one more of it and the other
less, and the column remembers which sections are collapsed and how much room each
has, as it does the widths of the three columns. Layers and Selection start open,
with the most room for the layers, and Problems starts collapsed.
Problems shows how many there are in its heading, red when any is an error, and opens
by itself for a draft that does not load. Every problem found in the files is listed in Problems with the path to it. A draft that does not load shows its
problems and draws nothing.

Clicking the card selects what is drawn on top at that point, the item the preview
outlines on hover, and clicking the empty space around the card lets go of the
selection. The layers list selects too, which is how to reach a layer beneath
another, and shows in
a dimmer type what the test card does not draw, with the reason on hover. Dragging
moves the selection, and the handles on a text box, an art window or a symbol
resize it. A layer moves but has no handles, since its image is drawn at its own
size. A text box set on its baseline is outlined where its text is, and its own place is
drawn as well: a dashed line across the box's width at its baseline, with a tick at each end.
Its height is not used, so it has no handles of an area. Dragging an
end of the line stretches the box along it, which writes the new X and width and leaves the
baseline where it is, and the selection reads as the line, with X, Width and Baseline, since
the numbers of the ink depend on the text. A box on its baseline snaps by its line, to the card's
lines and to the baselines of boxes marked as ones to snap to, and a guide says baseline
to the other box's baseline. A layout that turns the card makes the line a vertical one,
with its ends at the top and bottom. The Vertical choice of a text box also offers cap-top and
cap-center, which set the box's Y on the top or the middle of the text's capital letters and not
on its baseline, so a title that shrinks to fit stays centered with a mana cost on the same Y. The
line, its caption and the selection's third number are then named cap top or cap center. An edit that
sets either one names the engine release that reads them in the template's minEngine, raising an
older one, and undoing the edit puts it back. Edges and middles snap to the card's own lines within a few pixels, and to
another item's only when that item is marked as one to snap to. Holding Option or
Alt turns snapping off, and while a box is dragged the guide says which of its edges
is held and what lies on that line, which is outlined. The arrow keys move the
selection one pixel, and ten with Shift.

The view of the card has camera controls pinned to the bottom corner of the stage, which
stay there when the card moves: a Select tool and a Pan tool, zoom out and in, the zoom
as a percent, which cycles fit, 100% and 200% when it is pressed, and a button that fits
the card back in the middle of the view. Turning the wheel over the card zooms in or out
about the pointer, a trackpad's pinch does the same, and a sideways turn is left alone.
`+` and `-` step the zoom, and `0`, `1` and `2` are fit, 100% and 200% as in Single, 100%
being one of the picture's pixels for one of the screen's. Select, which dragging does to
begin with, selects, moves and resizes. With Pan chosen, dragging moves the card about the
view by any distance, and holding Space does the same for as long as it is held, while
the pointer is over the view, so Space still presses a button when it is anywhere else.
The middle button pans in either tool. A pan does not select or move anything or outline
what is under the pointer, and what is drawn on the card, the outlines, handles and guides,
goes with it. Fit puts the card back in the middle, and so does opening another template.

Three more controls sit in the same cluster. An eye shows and hides the overlays, the marks the
card draws for a selected item: the room a text box has left, what is in its way and the red
stripes, the placement line of a box that was moved or narrowed, and the caption of tags beneath the
outline. The selection's outline, a baseline and its ends to drag, the handles and the guides of a
drag stay, since they are how an item is edited, and so do the notes under the selection's name.
`O` does the same, and whether the overlays show is kept with the layout settings. Copy image puts the
picture of the card as it is drawn now on the clipboard as a PNG, and Save image asks where to put it
and writes the same picture as a PNG named for the card, in the folder the app saves renders to. It is
the picture the card shows, at the preview resolution, with anything the layer eyes have hidden or
forced left out or put in, and not a print resolution render, which Single makes. A window whose
webview cannot put an image on the clipboard says so and points to Save image. Both wait until a card
has been drawn, and a render that fails leaves nothing to copy or save.


Every change is saved to the layout file as it is made, with the file's order and
layout kept and only the numbers changed, and the card is drawn again a moment
later. Undo and Redo, in the layers heading and on `⌘Z` and `⇧⌘Z` (`Ctrl` on
Windows and Linux), take changes back and make them again, and a run of arrow keys
is one change. The numbers are the finished image's pixels, and the app turns them
into the layout's own when the layout turns the card. A layout that builds on
another with `extends` or `patch` can be looked at and not changed here, since its
numbers are not all in its own file.

Below the selection, its settings are fields in the right column. A text box has
its place and size, what it draws and when, its font role, size, color, alignment,
spacing, padding, tracking and shadow. A layer has its position, opacity, blend mode
and the conditions it draws under, and the slot its part reads. An art window has
its place, the layer it sits above and its rotation, and a symbol its place, alignment
and scale. A rotation is chosen in degrees, 0, 90, 180 or 270 deg. Canvas, under the
template's About in the left column, holds the canvas width and height in pixels, its
resolution in dpi and its rotation, in one Size group that belongs to the whole template,
with each unit written inside its field at the right. It starts collapsed and opens with its
own arrow like the sections of the right column. A bracket joins the width and the height
with a padlock on it, the padlock a layer's row has, drawn in the same color while it is
closed, and it is closed to begin with, so a change to one side changes the other to keep the
shape the canvas had, in one change that undo takes back. Opening the padlock lets the
two change on their own, and closing it again keeps the shape the canvas has then. It is
closed again after the page is reloaded, since it is not kept with the template. About holds
the template's name, description, author, license and credits. What is marked on the card for a selected text box is named in a caption set beneath its outline,
where none of its text is: baseline, what the card did to the box, and the layer its text keeps
out of. Beneath the selection's name are the notes worth knowing about a text box for the card being
tested. A box the card moved to another box's row or narrowed to stay clear of one is marked where
it ends up, by a rectangle, or for a box on its baseline by the line it ends up on drawn over the
box's own line so that what the card gave up shows past its end, with the box it was for taken from its flow, an override of the box that is in force
is named by its condition, since the fields show the box as it is written, and text that shrank to
fit is said with its size against the one the box asks for, in the warning color when it does
not fit even at the smallest size.

A text box's Flow group says which layer its text steps around, which box it stays clear of
and how large a gap it keeps, and which layer it moves for. The card shows the room a selected
text box has in three colors. Purple is the room its text takes, the outline every item has. A
blue ghost is the room it has left, beside its text for a box on its baseline, from the text to the
end of its line as the card left it, which is a narrower one when the card narrowed the box, and on
both sides of text that is centered, and the whole box, tinted, for a box with an area. Red is what
is in its way: the layer it steps around, the box it stays clear of and the layer it moves for are
outlined in red, and where the room the box has as the layout gives it meets such a thing, which it
cannot use, is striped in red. The red outlines are there in the other direction too: select the
layer a box steps around, or the box another stays clear of, and the boxes that do so are outlined
in dashed red and striped where their room meets the item. Red is not the yellow of the items a
drag snaps to, so all of it stays on the card while a box is dragged and follows it, and a
box moved on its baseline has the ghost and stripes move with its line. A note under the selection's
name says what the setting does for the card being tested: the text steps around the layer, or
has the whole box because the card does not draw the layer, or because the layer does not reach the box.
A number, a color or a
condition is checked as it is typed and its mistake shown under the field, a change
is made a moment after typing stops, and an empty optional field takes its setting
out so the box goes back to its default. The app checks every change against what
the engine accepts, and one it refuses, such as a condition it does not know, says
why and leaves the file alone. A copy keeps its original's license and the credits
it came with, and the fields do not change them.

Edit JSON opens the template's files under the preview, one tab for `template.json`
and one for each layout. The text is checked as it is typed, each mistake in the JSON
is shown with its line and column, and a mistake in what it means is shown with the
path to it. Text that is JSON and would leave the template loading is saved as typed,
with its layout kept, and text that would not is left unsaved until it is fixed.
A template that does not load can be fixed here. A change made with a lever or on the
preview shows in the open file, unless text typed there is waiting to be saved.

A layer shows the images of its part as a grid with a cell for each material its slot
reads, a color, a pair of colors, gold, a land and so on. A cell with an image shows it
small and its size, and Replace writes the chosen PNG over it, so the layer keeps its
place and condition. A material with no image of its own says what draws it instead,
the images of its colors blended for a pair or the part's `any` image, and a material
that nothing draws is outlined as a hole a card of that material would fall into.
Images are checked as they come in. A file that is not a PNG, or is far larger than a
frame is, is refused with the reason, and one that is larger than the canvas, is a
different size than the part's others or has no transparent pixels where a frame
layer is usually cut out is taken with a warning. Under About, Fonts adds
a `.ttf` or `.otf` file to the template, which keeps it in its `font` folder with its
hash and the names it carries, and Text roles choose for each role the font it
draws with, one of the template's or one of the engine's own.

Each row of the layers list has what acts on that item. A layer has a grip, three
bars, on its left, and dragging it by the grip moves it in the stack, with a line
showing where it will land. The same is on the keyboard as Alt with the up and down
arrows on the row, and as Move up and Move down in its menu. An icon says what the
item is, text, an art window, a layer or the set symbol, and the name beside it
selects the row. A padlock locks the item, faint while it is open and in the full text
color, and closed, once it is locked, and a grid makes it a line that a dragged item
snaps to, crossed out and faint while it is not. The three dots at the right open a
menu with Duplicate, which copies the item a little way down and to the right, and
Delete, which takes it out of the layout and leaves its part and images where they
are. A symbol is moved and resized and has no menu to copy or delete it.

An eye at the left of each row leaves the item out of the card's preview, or puts it back,
and the card is drawn again without it. It changes only the preview: nothing is written to the
template, it is not undone with the edits, and a template that is opened again starts with
everything showing. A hidden item's name is struck through, it cannot be picked on the card, and
the list still reaches it. The eye of a layer that the card does not draw, because its condition
does not hold, is off, and pressing it draws the layer whatever the condition says, marked in the
accent color, with the layout described again so the layer has a place on the card to be picked
and text still steps around it as it would. Pressing it again lets go. Anything else the card does
not draw has an eye that cannot be pressed. The strip of coverage cards ignores both, since it shows the
template as it is.

The plus beside the layers adds a text box that draws something the card has, an art
window above a layer, or a layer for a part the template has or for a new part whose
first image is chosen then. Each of the changes in the list is one that undo takes
back, and a layer for a new part is undone with its part and image in one step. The app
holds the files an undo needs in memory, up to a limit, and an undo that needs one it
has let go of says so and leaves the draft as it is.

A locked item cannot be picked, hovered over, moved or resized on the preview, and the
arrow keys leave it where it is. A click where it lies reaches what is beneath, and
`L` locks and unlocks the selection. It stays in the list, and its fields and the
list still change it. Items a drag snaps to are marked the same way, and none are
marked to begin with. Both are kept with the draft, beside its test card, and are not
part of the template.

Three buttons at the bottom of the right column finish a template. Use in mimic builds
the draft into a template and keeps it among the installed ones as `user.<name>` at the
version `draft`, so Single, List and Run can draw with it once it is chosen in Templates,
where its version is listed as a draft from the editor and can be copied back into the
editor like any other. Using it again after more edits replaces what was kept, and when
the template in use is that one it is opened again, so the next card shows the change. A
run in progress keeps the template it started with, so the button waits until it ends.
Deleting the template in the editor takes it back out of the installed ones, and the
default template is in use if the deleted one was. Export asks first, in a dialog that
states the name, id, version, author and license, lists every credit, including the one
a copy keeps that names its original, and lists what stops the template being built, and
then asks where to save a `.mimic` file named after the template and its version. The
bundle is the same one Use in mimic keeps, with its license and credits. Submitting it
to the template catalog stays a step done by hand. Show folder opens the draft's folder in
the system's file browser, for editing a file in another tool.

Save as a test card, under the presets, keeps the card being tested, with the changes made
to its fields, under a name of up to 40 characters, in `testcards.json` beside the drafts.
It is offered in every draft after the six presets, needs no lookup and works with no
network, and a small cross beside it takes it out of the list again. Up to 24 can be kept.
The built-in presets cannot be removed.

Coverage, under the card's view, draws every test card, the presets and the ones saved,
small and without their art through the draft as it is, one after another, when Draw every
test card is pressed. Each tile says whether its card was drawn, and one the template could
not draw says why, so a material with no image shows as a card drawn with a hole or as
a tile with a reason. The line above the tiles counts the cards drawn and names those
that were not. Choosing a tile tests the template with that card. An edit to the template
leaves the tiles in place, dimmed, with the line saying the template has changed, until
they are drawn again, and opening another template clears them.

## Features that are not built yet

Features that are designed but not built still appear, dimmed with a lock, and
hovering or focusing one says why. The app decides this: `Settings.Capabilities`
returns every feature key with a state of `live`, `planned`,
`needs-engine` or `needs-template` and a reason, and the page renders what it is
told. Lifting a gate is a change to the table in
`internal/services/settings/capabilities.go`. Every gate that remains names the
GitHub issue that tracks what it waits on.

## Settings

Settings persist in `prefs.json` under the per-OS user config directory, in a
`mimic` folder beside the template cache. The file is plain JSON, and editing it
by hand while the app is closed is supported.

Card data, in the Sources group, chooses where lookups read cards from. Scryfall
API is the default. Local copy downloads Scryfall's
[bulk data](https://scryfall.com/docs/api/bulk-data), about 100 MB, and keeps a
trimmed copy of every printing in a `scryfall` folder beside `prefs.json`, about
70 MB. With it, a list of a hundred cards resolves in well under a second and
Single's printing picker reads from disk. Lines naming a card the copy does not
have, usually one printed after the download, still go to the API, and so do
`?query` lines and Single's search box, since those use Scryfall's search
syntax. Art comes from Scryfall's image host either way. Scryfall refreshes the
files daily, and the panel offers an update once a copy is a week old or a
newer one is out.

Fonts, also in Sources, lists each text role of the installed templates with a
picker for the font it draws with. The picker offers the template's own font and
every font in the folder, and a choice is kept by Save. A role whose chosen font
is no longer in the folder says so and draws with the template's own. Add font
copies a `.ttf` or `.otf` into a `fonts` folder beside `prefs.json`, and Remove
deletes one. Those write at once rather than on Save, and the next render uses
them. When `-fonts` or a checkout's `local-fonts/` is in use instead, the panel
lists that folder's fonts and cannot add or remove them. Fonts added by an
earlier version, one folder for each role, are moved up into the folder and
assigned to their roles the first time the app starts. The standard roles and the
fonts real cards use are listed in the
[top-level README](../README.md#templates-and-fonts).

## Scryfall rate limits

Calls to the Scryfall API are paced to Scryfall's
[published rate limits](https://scryfall.com/docs/api/rate-limits): two a second
for search and named lookups, and ten a second for everything else. If Scryfall
still answers 429, every call waits out the 30 seconds it asks for. Resolving a
list makes one search per matched line, so a 100-line list takes about a minute
the first time and is cached for the rest of the session, unless a local copy of
the card data is in use. Card images come from Scryfall's image hosts, which
have no limit, so they skip the queue.
