# Releasing the app

This is what to check before a ui release, and how a release is cut. The Go and
frontend tests run in CI on every change. The window itself, the installers and
the updater are the parts they cannot reach, so they are checked by hand from the
installers that a packaging run builds.

## Before a release

Run the packaging workflow on the release commit (Actions, package-ui, Run
workflow, publish left off). It builds all three installers and uploads them as
workflow artifacts. Install from those files on each system, not from a local
build, and work through the list below.

All systems:

- The installer or bundle opens, and the unsigned-app warning matches the
  top-level README.
- Single: search, select a card, edit a field, render, zoom and pan the preview,
  and Save through the system dialog.
- From a List: open a file, drop a file, paste, resolve, render a batch into a
  chosen folder, Stop, and Retry failed.
- From a List: open the row inspector with `e`, edit a field, Open in Single and
  Apply to row, and use each item in the Actions menu, including undo after a
  removal.
- Templates, Overrides, Global: add a rule, see its match count, reorder it, turn
  it off, then save a preset, change a setting and apply the preset back. Render a
  card and confirm the rule's field is set and the report records it.
- MPC Autofill output and the cardback upload.
- A fresh config folder: the top bar says "No template installed", the strip
  under it leads to the Templates tab, and Single and List say no template is
  installed in place of rendering.
- Templates: download `mimic.m15` (about 234 MB), then confirm its layouts are
  listed, its title shows in the top bar, and a card renders. On the Defaults
  tab, set a face default for split and transform cards and render one of each.
- Settings: download the local card data, add and remove a font, and confirm the
  theme follows the system.
- The window opens where it was left, including after unplugging a display.
- Starting mimic a second time brings the first window forward.
- Closing during a run asks before quitting, and agreeing stops the run.
- The update strip appears against an older version, and Install replaces the app
  and restarts into the new version.
- The menus, the keyboard shortcuts and the tooltips behave as the shortcut
  reference says, including a tooltip that must not appear for a control the
  pointer left before the delay.
- Uninstalling removes the app and leaves the config folder alone.
- A config folder from 0.9.2 keeps its prefs and card data, and starts with no
  template. Copy a real 0.9.2 config folder into place, then start the build.
  A saved choice of `normal`, `split` or `transform` is rewritten to
  `mimic.m15`, the cached bundles of those three are deleted, and the app shows
  "No template installed" until `mimic.m15` is downloaded.

On Windows the installer must run without administrator rights and add the WebView2
runtime on a machine that lacks it. On Linux, check the Flatpak bundle (it installs, launches and renders a card, and its
file dialogs open), and the `.deb`, and that a package-managed install offers the release page for an
update instead of replacing itself.

## Throughput

The run loop is compared against the previous release on one machine with the
same 100 cards. A comparison is like for like only when both builds pin the same
engine, so say which engine each build used when quoting a result.

`cmd/throughput` renders a JSON file of run rows through the run service and
prints cards per second for each pass. It renders with an installed template, so
install `mimic.m15` first from the Templates tab. Take the rows from the local card data, one hundred distinct English cards
with art, and run it three times. The first pass fetches art and the later ones
measure rendering.

```
go build -o /tmp/throughput ./cmd/throughput
/tmp/throughput -rows rows.json -runs 3
```

For the earlier release, extract its tag with `git archive ui/v0.9.2 ui`, build it
with `GOWORK=off go build`, start it with `-no-open`, post the same rows to
`/api/run` and poll `/api/run` until it finishes. Sample the process's resident
memory while it runs.

The 1.0 comparison, on an Apple M3 Max with 16 workers, JPEG output and 1200 dpi
cards, was measured when both builds pinned the same engine. The current build
pins engine v0.18.0, which blends pairs from single color art and caches the
result, so these numbers come from the earlier engine and a new run against them
is not like for like:

| Build      | Cards per second                   | Peak memory      |
| ---------- | ---------------------------------- | ---------------- |
| 0.9.2      | 18.2 to 19.9, and one pass at 13.2 | 16.7 to 17.2 GiB |
| This build | 20.5 to 25.0                       | 14.5 to 15.6 GiB |

The window adds three WebKit processes beside the app. With the page open and
idle they used about 110 MiB, and a finished card decodes to about 58 MiB while it
is shown. The resource model reserves 512 MiB for them before it sizes a run.

The measurement leaves out the page drawing a run's progress. During the check on
each system, start a 100 card list and confirm the progress stays smooth and the
final rate in the run's summary is close to the rate above for that machine.

## Cutting a release

release-please keeps a pull request open for the ui release and updates it as
changes land. The first release is 1.0.0, which a `Release-As: 1.0.0` footer asks
for, since a breaking change from 0.9.2 would otherwise produce 0.10.0. Merging
the release pull request tags `ui/v1.0.0`, and the release workflow calls the
packaging workflow to build and attach the installers, `SHA256SUMS` and the
signatures when the signing key is set.

The release pull request body is generated from the commits, so it cannot
describe behavior a user needs to know before upgrading. When a release changes
what an existing config folder does, open the GitHub release with one sentence
on it.

Afterward, install the published release on one system, check that the updater
sees it as current, and that an older build offers it.
