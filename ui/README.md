# mimic ui

A desktop app for the mimic card renderer. It opens a window that shows an
embedded web frontend through the system webview, searches Scryfall, previews a
card through the normal template, and saves the result. The frontend and the Go
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

- `-fonts <dir>` renders with the font overrides in dir, laid out one subfolder
  per role, ahead of every other font source.
- `-smoke` runs the launch check against a scratch config folder and exits. It
  opens the window, boots the page, renders a card through the bridge, loads the
  images, drives a few flows in the page (a popover, the settings dialog, a
  rendered list and the Single preview) and exits zero when all of that works.

A build made with `-tags dev` turns on the webview's developer tools.

## Using it

The page is one shell with a mode rail down the left: Single, From a List, and
From Your Art make cards, Templates holds everything about the frames cards
render with, and Run keeps the record of a batch. The active template's name
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
`ARCH=arm64`, and makes a per-user installer and a portable `.zip`. Linux makes
an AppImage, a `.deb` and an `.rpm` for its own architecture. The AppImage step
downloads linuxdeploy when it runs.

`.github/workflows/package-ui.yml` runs all five on native runners. A release
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
going a dialog asks first and stops the run if you agree. Starting Mimic a
second time brings the first window forward instead of opening another.

The menu bar follows each platform. File holds Save Card, Open List and Choose
Output Folder, View switches modes, and Help links to this repository and shows
the versions in About. On macOS the menu owns `⌘S` and `⌘1` to `⌘5`. On Windows
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

Fonts, also in Sources, lists each font role with the font it draws with and
whether that is yours or a default. A role whose file could not be read says
why and stays on its default. Add or Replace stores a `.ttf` or `.otf` for that
role in a `fonts` folder beside `prefs.json`, and Remove returns the role to
its default. These write at once rather than on Save, and the next render uses
them. When `-fonts` or a checkout's `local-fonts/` is in use instead, the panel
shows that folder read-only. The role folders and the fonts real cards use are
listed in the [top-level README](../README.md#templates-and-fonts).

## Scryfall rate limits

Calls to the Scryfall API are paced to Scryfall's
[published rate limits](https://scryfall.com/docs/api/rate-limits): two a second
for search and named lookups, and ten a second for everything else. If Scryfall
still answers 429, every call waits out the 30 seconds it asks for. Resolving a
list makes one search per matched line, so a 100-line list takes about a minute
the first time and is cached for the rest of the session, unless a local copy of
the card data is in use. Card images come from Scryfall's image hosts, which
have no limit, so they skip the queue.
