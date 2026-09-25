# mimic engine

The engine renders a Magic: The Gathering card from Scryfall data into a PNG. It
is a Go module with three layers.

`card` fetches a card's printed information from Scryfall and holds it as a plain
`card.Data`. `Client.FetchByName` does a fuzzy single-name lookup,
`Client.Search` runs a query through Scryfall's full search syntax and returns a
page of matches, and `Client.FetchArt` downloads a card's art crop. Responses
are bounded in size and time so a slow or oversized reply cannot stall a render.
For Scryfall data from elsewhere, such as a line of a bulk data file,
`card.FromScryfallJSON` maps one card object the same way a lookup does, and
`card.ArtCropURL` builds a card's art crop address from its Scryfall id.

`template` turns a `card.Data` and its art into a finished image. A `Template`
reads its frame layers and geometry through an `AssetProvider`, lays out and
draws the text, and returns a pixel buffer. `template/normal` is the modern
frame, and `template/transform` is the same frame for both faces of a transform
card. `RenderTextBox` handles wrapping, shrink-to-fit, and inline mana symbols.

A template only draws the cards its frame was made for. `template.Classify`
gives each image a card renders to a role, where the face sits on the card
(`single`, `transform_front`, `mdfc_back`, `split`, and so on), and a kind, the
frame it needs (`standard`, `planeswalker`, `saga`, `basic_land`, and so on).
Each template registers the roles and kinds it `Supports`, and a template from
`template.Get` refuses any other face with an `*UnsupportedError` before
drawing. A double-faced card renders as two images, chosen with
`RenderRequest.Face`, and a manifest's layers and text boxes can name the
`front` or `back` condition to draw on one face only.

`mpcfill` lays finished cards out as an MPC Autofill project, a folder of
PNGs plus the `cards.xml` order file its desktop tool and website import. Its
README explains the file-naming and folder rules MPC Autofill forces on it.

`cmd/rendercard` is a command that wires the two together: it looks up a card by
name and writes a PNG, and serves as a reference for other front ends.

## Using it

```
go run ./cmd/rendercard -name "Lightning Bolt" -o bolt.png
```

With no `-assets` directory it generates placeholder frame layers, so the command
runs without a local asset set. Point `-assets` at a real template directory for
finished frames, and `-fonts` at a directory of font overrides. `-face 1` renders
a double-faced card's back face with its own art.

Engine releases are Go module tags and carry no prebuilt binaries. To install
the command outside a checkout, use `go install` instead:

```
go install github.com/odevine/mimic/engine/cmd/rendercard@latest
```

A binary installed this way is unstamped, so it renders any template bundle
regardless of the engine version the bundle requires.

Card data comes from Scryfall (https://scryfall.com). Respect their API
guidelines when fetching at volume.
