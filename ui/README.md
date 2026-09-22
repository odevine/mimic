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

The page has three panes. The left pane searches Scryfall with its full query
syntax (for example `t:goblin c:r cmc=1`) and lists the matches; the search box
remembers recent queries and offers them as you type. Selecting a match fills
the center pane with the card's fields, which you can edit. Render preview draws
the current values through the normal template into the right pane, and Save PNG
downloads that image.

A card's art is fetched once when you select it and reused across edits, so
tweaking a field re-renders without another download. With no local template
assets present the app falls back to placeholder frame layers, the same as the
`rendercard` command in the engine module.
