# mimic ui

A local app for the mimic card renderer. It runs a loopback HTTP server, serves
a small embedded web frontend, and opens it in your browser. It searches
Scryfall, previews a card through the normal template, and downloads the result.

The whole binary is pure Go: the frontend is embedded with `//go:embed`, so
there is no Node or npm step, and the module builds with `CGO_ENABLED=0` and
cross-compiles to a single static file per target.

## Running it

```
go run .
```

Run it from a checkout of the full repository, not the `ui` directory in
isolation, so the Go workspace resolves the local `engine` module. It prints the
URL it bound and opens that page in your default browser.

Flags:

- `-addr` binds a specific address instead of the default loopback ephemeral
  port, for example `-addr 127.0.0.1:8080`.
- `-no-open` prints the URL but does not open a browser.

## Using it

The page is one shell with a mode rail down the left: Single, From a List, and
From Your Art make cards, while Overrides and Run hold the rules and the record
of a batch. The active template sits in the top bar as a dropdown, and settings
and a keyboard reference sit beside it.

Single is the mode that works today. The left column searches Scryfall with its
full query syntax (for example `t:goblin c:r cmc=1`) and lists one row per card
name. Selecting a match fills the editor, where a printing dropdown switches
between every printing of the card. Fields you have not edited follow the new
printing, and edited ones keep your value. Each edited field carries a dot and
a revert control. The mana cost and rules text show their braced codes as the
engine's own pips as you type, with a code the engine does not draw flagged,
and a palette inserts symbols for anyone who does not know the syntax.

Render draws the current values into the preview, which zooms to fit, 100% and
200% and pans by dragging. Save downloads the card at the output resolution.
`⌘↵` renders, `⌘S` saves, `/` jumps to search, and `?` lists the rest.

A card's art is fetched once when you select it and reused across edits, so
tweaking a field re-renders without another download. With no local template
assets present the app falls back to placeholder frame layers, the same as the
`rendercard` command in the engine module.

## Features that are not built yet

Features that are designed but not built still appear, dimmed with a lock, and
hovering or focusing one says why. The server decides this: `GET
/api/capabilities` returns every feature key with a state of `live`, `planned`,
`needs-engine` or `needs-template` and a reason, and the page renders what it is
told. Lifting a gate is a change to the table in `capabilities.go`.

## Settings

Settings persist in `prefs.json` under the per-OS user config directory, in a
`mimic` folder beside the template cache. The file is plain JSON, and editing it
by hand while the app is closed is supported.

## HTTP API

The page drives the server entirely through a small JSON API on the loopback
address, so a script can drive it too.

| Endpoint                           | Purpose                                              |
| ---------------------------------- | ---------------------------------------------------- |
| `GET /api/search?q=`               | Scryfall search, one result per match                |
| `GET /api/printings?name=`         | Every printing of one card, newest first             |
| `GET /api/recents`                 | Recent searches                                      |
| `POST /api/render`                 | Start a render of a base card plus edits             |
| `GET /api/render/{id}/events`      | Render progress as server-sent events                |
| `GET /api/render/{id}/image`       | The finished PNG, `?download=1` for an attachment    |
| `GET /api/symbol?code=&px=`        | One mana symbol as a PNG, 404 for an unknown code    |
| `GET, POST /api/resolution`        | Preview and output resolutions                       |
| `GET, PUT /api/settings`           | Interface settings                                   |
| `GET /api/capabilities`            | The feature gate map                                 |
| `GET /api/templates`               | Template catalog rows                                |
| `GET /api/template/active`         | The active template and where its assets come from   |
| `POST /api/template/select`        | Switch template, downloading first when needed       |
