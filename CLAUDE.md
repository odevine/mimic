# Working in this repository

This file is for AI coding sessions working in mimic. It records how the repo is
laid out, how changes are checked, and the conventions a change is expected to
follow. When a human contributor's instructions conflict with it, theirs win.

## Layout

The repository holds two Go modules that are versioned and released separately.

| Directory      | Contents                                                              |
| -------------- | --------------------------------------------------------------------- |
| `engine/`      | The renderer library and the `rendercard` command                     |
| `ui/`          | The desktop app: Wails v3 shell, Go services and an embedded frontend |
| `local-fonts/` | A drop folder for real card fonts, which are not tracked              |

`ui` depends on a released `engine` through the pin in `ui/go.mod`. Frame
templates live in the separate `mimic-templates` repository, and layer
compositing in `impasto`. A change that needs either belongs there, not here.

To work across `engine` and `ui` at once, make a local `go.work` with
`go work init ./engine ./ui`. It is ignored by git. Build and test with
`GOWORK=off` when a result must reflect the pinned engine, as CI does.

## Commands

Run these from `ui/` before handing a change over. Add `-tags gtk3` to the Go
commands on Linux.

```
gofmt -l .
go vet ./...
go test -race ./...
node --test frontend/test/*.test.mjs
go run . -smoke
go tool wails3 task test:e2e
```

`-smoke` opens the real window against a scratch config folder, boots the page,
renders a card through the bridge, drives a few flows through the page's own
buttons and fields (a gated control's popover, the settings dialog, a list of one
custom card rendered to a folder, and Single's preview) and exits. The same page
is served by the test server as `/smoke.html?e2e`, and `tests/smoke.spec.mjs`
runs it in headless browsers, so the flows are checked on every change and not
only on CI's real webviews. For the engine, run `go vet ./...`
and `go test ./...` in `engine/`. After a change to a workflow in
`.github/workflows/`, run `actionlint` on it, because a file that parses as YAML
can still be one GitHub refuses to run. A job that fails in zero seconds, named
after the workflow path, means the file is invalid.

`test:e2e` drives the real page in headless Chromium and WebKit against a test
build of the app and a fake Scryfall, with no window and no input control. Run
`go tool wails3 task test:e2e:install` once first. Pass Playwright's arguments as
`ARGS='-g "name"'`, since the task runner takes variables and not flags. To look
at the page, `test:e2e:shot` writes a PNG from a given search and state and
prints its path, and `dev:server` serves the page on a local port. The tests
live in `e2e/tests`, with a page object per mode in `e2e/pages`.

The page tests in `e2e/tests/page` need no server. They load the real frontend
in a browser against a bridge the test controls (`e2e/bridge.mjs`), which
answers each call into the app from a reply the test sets and sends the page its
events, so they reach states the real app makes hard to reach, such as a render
that fails, an update that errors or a lookup that stops partway. They start from
replies captured from the real app in `e2e/bridge/replies.json`, and a test fails
on a call no reply covers. `test:e2e:page` runs only them, in a few seconds. When
a test in `e2e/tests/bridge-replies.spec.mjs` reports the captured replies are
stale after a change to what the bridge returns, run `test:e2e:capture`. Reach
for a page test when a state needs a reply the fake upstream cannot produce, and
for a test against the test server when the real services should answer.

Each test gets the fake upstream, a scratch dialogs folder and a guard that fails
it if the page reaches outside the machine, through the fixtures in
`e2e/fixtures.mjs`. The `fake` fixture changes what Scryfall, GitHub and the
template catalog answer (a release, a catalog of bundles, a failing or slow
host), and `scratch` reads what the app saved and offers it files to open. Tests
in `e2e/tests` share one server and leave its settings, rules, presets and fonts
as they found them. A test that installs a template or otherwise changes what the
app keeps for good goes in `e2e/tests/isolated`, which has a server of its own
and does not retry. Card fixtures are in `testassets/scryfall`, and a search the
fake does not understand fails loudly, so add the card or the operator rather
than loosening a test.

For a question only the real webview can answer, such as how WKWebView lays out
a control, `dev:mcp` builds the app with the `mcp` tag and opens its real window
against the same fake Scryfall and a scratch folder at `bin/mcp-home`. Start it in
the background and tell the developer a window is about to open, since it appears
on their desktop. `mcp:call` then sends one tool call to it and prints the
result, as in `go run ./cmd/mcpcall dom_query selector='#results .result'`, and
`go run ./cmd/mcpcall tools` lists every tool and its arguments. The tools
dispatch DOM events inside the page, so they need no accessibility permission and
no control of the mouse or keyboard. Call services through `js_eval`, whose
argument is named `js`, and the page's own `api.js`, not `call_bound_method`,
which stops the page's event handlers (wailsapp/wails#6136). A selector that
matches nothing comes back as a stack trace and not a message. Stop the build
with `go tool wails3 task mcp:stop`. It never runs in CI, and a hand-off says
what was seen this way and what was seen only in the test server.

Do not drive the desktop window with accessibility or UI scripting. Check
behavior through tests, logs, `-smoke` and files the app writes, and say plainly
in the hand-off what was not seen in a real window.

## Architecture rules

cgo and Wails live only in `ui/internal/desktop`. Every other package must build
and test with `CGO_ENABLED=0`, and CI fails an import of Wails from anywhere else.

The page reaches Go only through the services wrapped in
`internal/desktop/bound.go`. A method is callable from the page only if the
wrapper type lists it, so a new service method needs a wrapper entry, a matching
call in `frontend/js/api.js`, and passes the bindings test. Errors cross the
bridge as `apierr` kinds.

The bridge golden, `internal/desktop/testdata/bridge.golden`, records every bound
method and the JSON shape of the types it uses. After a change to a bound method
or to a type that crosses the bridge, run
`go test ./internal/desktop -run Bridge -update` and review the diff, since it
shows exactly what the page will now receive. A type with its own `MarshalJSON`
needs a stand-in in `bridge_test.go`.

The frontend is vanilla ES modules with no bundler, no npm step and no runtime
dependencies. State goes through the signals in `js/state.js`. Features that are
not built appear gated by `Settings.Capabilities` in
`internal/services/settings/capabilities.go`, and every gate that is not live
must name its tracking issue as `#123`. Never write a version number in a gate.

Calls to Scryfall go through the paced client, and a release opens no listening
port. Do not add a server mode to the product.

A build with the `server` or `mcp` tag is a test build and never ships. The
`server` tag runs the app as a headless HTTP server on `WAILS_SERVER_PORT`, and
`mcp` adds a loopback MCP endpoint to the real window. Both set
`buildinfo.TestBuild`, and `main.go` then calls `testbuild.Apply`, which reads
three environment variables. `MIMIC_E2E_HOME` names a scratch folder used in
place of the user config folder and is required. `MIMIC_E2E_UPSTREAM` is the
`host:port` that answers every outbound request, with the original `Host` header
kept, and every request is refused when it is unset. `MIMIC_E2E_CAPABILITIES`
lists gate keys to report live. A test build also ignores loose template assets,
takes no single instance lock, and answers file dialogs, questions and links from
the `dialogs` folder inside the scratch folder, so a save lands in
`dialogs/saved` and nothing opens on the desktop. Release recipes build with the
`production` tag, and combining it with a test tag fails to compile.

## Branches, commits and releases

Cut a new branch with `--no-track` from the up-to-date base, as in
`git switch -c feat/name --no-track origin/main`. A branch that tracks
`origin/main` lets a plain push land on main.

Commits and pull request titles use Conventional Commits with a scope of `ui` or
`engine`, for example `feat(ui): add global override rules`. release-please builds
the changelog and version from them, so `feat` and `fix` are releasable and
`docs`, `chore`, `test` and `ci` are not. Keep engine and ui changes in separate
commits. Breaking changes use `!` and a `BREAKING CHANGE:` footer. End commit
messages and pull request descriptions with the attribution lines the session
gives.

## Writing

Code comments explain what the code does or a non-obvious reason it does it.
They do not narrate history, so leave out "formerly", "extracted from", "phase 2"
and the like, which belong in the commit message. Do not use semicolons or
dashes in comments, do not end a comment with a period, keep it to one line
unless it needs more, and skip comments that restate the code.

Documentation, commit messages and pull request descriptions open by explaining
the thing, not labelling it. Prefer short paragraphs of prose to bullets for
conceptual material, and keep tables for tabular data. Use no em or en dashes.
State technical facts plainly and hedge editorial claims. Do not put other tools
down, use superlatives, or write slogans. Do not cite a document the reader
cannot open, such as a private design doc, and verify any URL you cite.

## Tests

New behavior comes with a test next to the code. Go tests use the fixtures in the
`*test` packages (`carddatatest`, `catalogtest`, `pipelinetest`,
`workspacetest`) and run with the race detector. Frontend logic that does not
need a document is tested under `frontend/test` with the dependency-free Node
runner and the Wails runtime stubbed. When a bug is fixed, the test should fail
on the old code.

A new feature follows the structure that is there. Its logic lives in a service
or a package under `internal/` with a Go test and the fixtures above. Anything
that crosses the bridge comes with the golden update described under
architecture. Pure frontend logic, such as parsing, formatting or state, goes in
a module that imports nothing from the document so it can be tested under
`frontend/test`, and the part that builds elements stays thin.

Report results faithfully. If a check fails or was skipped, say so with the
output, and say which parts of a change were only exercised in tests.
