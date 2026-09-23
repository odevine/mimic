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

Single and From a List work today, with Run as the console for a batch. In
Single, the left column searches Scryfall with its
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

Render writes one PNG per card into the output folder, picked through a folder
browser in the footer and remembered with a short list of recent folders. A
file is named after the card and the printing it came from, as in
`Sol Ring [C21-263].png`, and rendering into the same folder again overwrites
it. A second copy of the same printing in one run gets a `(2)` suffix, and a
quantity is recorded in the run report rather than written as extra files.
Cards render two at a time by default, which the settings panel changes.

Run shows the batch as it goes: overall progress with an estimate, a line per
card with its time or its own progress, and the finished render of whichever
card is picked. A failed card opens to its error and the stage it failed at.
Stop skips every card still queued, and a card already rendering finishes
first. When the run ends, a `mimic-run-<timestamp>.json` report beside the PNGs
records each card's outcome, printing, file, and field overrides, along with
the template and resolution used. Retry failed starts a new run from only the
failed cards. The server keeps the latest run, so reloading the page picks it
back up, and the template cannot be switched while a run is going.

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

The server answers only requests addressed to `localhost` or an IP address,
which keeps another website from reaching it through DNS rebinding. It refuses
a POST or PUT whose `Origin` header names a different site. A script that sends
no `Origin`, such as curl, is let through.

Calls to the Scryfall API are paced to Scryfall's
[published rate limits](https://scryfall.com/docs/api/rate-limits): two a second
for search and named lookups, and ten a second for everything else. If Scryfall
still answers 429, every call waits out the 30 seconds it asks for. Resolving a
list makes one search per matched line, so a 100-line list takes about a minute
the first time and is cached for the rest of the session. Card images come from
Scryfall's image hosts, which have no limit, so they skip the queue.

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
| `POST /api/resolve`                | Parse a list and start resolving it                  |
| `GET /api/resolve/{id}/events`     | One resolved row per event                           |
| `POST /api/run`                    | Start a batch into an output folder                  |
| `GET /api/run`                     | The latest run, 204 when there has been none         |
| `GET /api/run/{id}/events`         | Per-card progress and log lines                      |
| `GET /api/run/{id}/image/{n}`      | One finished card, read from the file it wrote       |
| `POST /api/run/{id}/stop`          | Skip the queued cards and end the run                |
| `POST /api/run/{id}/retry`         | Start a new run from the failed cards                |
| `POST /api/run/{id}/open`          | Open the run's folder in the file browser            |
| `GET /api/fs/list?path=`           | Subfolders of a folder, for the folder picker        |
