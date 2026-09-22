# mimic ui

A desktop app for the mimic card renderer, built with Fyne. It searches
Scryfall, previews a card through the normal template, and saves the result to
disk. It depends on the `engine` module's public packages and is joined to the
repository's Go workspace.

## Running it

```
go run .
```

Run it from a checkout of the full repository, not the `ui` directory in
isolation, so the Go workspace resolves the local `engine` module.

The window has three panes. The left pane searches Scryfall with its full query
syntax (for example `t:goblin c:r cmc=1`) and lists the matches; the search box
remembers recent queries and offers them as you type. Selecting a match fills
the center pane with the card's fields, which you can edit. Render preview draws
the current values through the normal template into the right pane, and Save PNG
writes that image to a file you choose.

A card's art is fetched once when you select it and reused across edits, so
tweaking a field re-renders without another download. With no local template
assets present the app falls back to placeholder frame layers, the same as the
`rendercard` command in the engine module.
