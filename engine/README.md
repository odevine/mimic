# mimic engine

The engine renders a Magic: The Gathering card from Scryfall data into a PNG. It
is a Go module with three layers.

`card` fetches a card's printed information from Scryfall and holds it as a plain
`card.Data`. `Client.FetchByName` does a fuzzy single-name lookup,
`Client.Search` runs a query through Scryfall's full search syntax and returns a
page of matches, and `Client.FetchArt` downloads a card's art crop. Responses
are bounded in size and time so a slow or oversized reply cannot stall a render.

`template` turns a `card.Data` and its art into a finished image. A `Template`
reads its frame layers and geometry through an `AssetProvider`, lays out and
draws the text, and returns a pixel buffer. `template/normal` is the modern
frame. `RenderTextBox` handles wrapping, shrink-to-fit, and inline mana symbols.

`cmd/rendercard` is a command that wires the two together: it looks up a card by
name and writes a PNG, and serves as a reference for other front ends.

## Using it

```
go run ./cmd/rendercard -name "Lightning Bolt" -o bolt.png
```

With no `-assets` directory it generates placeholder frame layers, so the command
runs without a local asset set. Point `-assets` at a real template directory for
finished frames, and `-fonts` at a directory of font overrides.

Card data comes from Scryfall (https://scryfall.com). Respect their API
guidelines when fetching at volume.
