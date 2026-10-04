<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/banner-dark.svg">
  <img src=".github/banner-light.svg" alt="mimic">
</picture>

mimic renders Magic: The Gathering cards from Scryfall data into PNGs. It looks
up a card, draws its name, type line, rules text, mana symbols, and art through
a frame template, and writes the finished card. Any field can be edited before
it renders, so the same tool makes a faithful proxy of a real printing or a card
that was never printed.

It runs as a local app. The `mimic` binary starts a small server on your
machine and opens its interface in your browser. From there you can render one
card at a time, paste a decklist or CSV and render the whole thing into a
folder, and manage the frame templates cards render with.

## Getting it

Each ui release on the [releases page](https://github.com/odevine/mimic/releases)
carries a prebuilt binary for macOS, Linux, and Windows, on both amd64 and
arm64. Unpack the archive for your platform and run `mimic`. It prints the
address it bound and opens that page in your default browser.

Frame templates download from inside the app, under Templates. Until one is
installed, cards render with placeholder frame layers.

## Building from source

Building needs Go at the version named in `ui/go.mod`. The app is pure Go with
its frontend embedded, so there is no Node or npm step, and it builds with
`CGO_ENABLED=0`.

```
git clone https://github.com/odevine/mimic
cd mimic/ui
go run .
```

## How the repository is laid out

The repository holds two Go modules, each versioned and released on its own.

| Directory      | Contents                                                        |
| -------------- | --------------------------------------------------------------- |
| `engine/`      | The renderer: Scryfall lookups, templates, text layout          |
| `ui/`          | The `mimic` app: the local server and its embedded web frontend |
| `local-fonts/` | A drop folder for real card fonts, which are not tracked        |

The [engine README](engine/README.md) covers the rendering library and the
`rendercard` command, and the [ui README](ui/README.md) covers the app, its
settings, and the JSON API it serves.

`ui` depends on a released `engine` through the pin in its `go.mod`, and CI
builds against that pin. To work across both modules at once, create a Go
workspace at the repository root. The `go.work` file is ignored, so it stays a
local convenience.

```
go work init ./engine ./ui
```

Releases are cut by release-please from conventional commit messages, and tags
carry the module name, as in `engine/v0.10.0` and `ui/v0.8.1`. Only ui releases
ship binaries.

## Templates and fonts

Frame templates are built in the
[mimic-templates](https://github.com/odevine/mimic-templates) repository and
published there as versioned `.mimic` bundles. The app reads that catalog to
list what is available and downloads a bundle when you select it.

The engine embeds open-license fonts, so it renders without anything extra. The
fonts printed on real cards are copyrighted and not redistributed here. If you
have them, add them in the app under Settings, Fonts. Each font goes to one
role, and a font added to the wrong role draws in the wrong place, so match the
file to the role in this table.

| Role           | Folder         | Draws                         | Real cards use          | Default when empty   |
| -------------- | -------------- | ----------------------------- | ----------------------- | -------------------- |
| Title          | `title/`       | card name, type line, P/T     | Beleren Bold            | Big Shoulders Bold   |
| Rules          | `body/`        | rules text, copyright line    | MPlantin                | Merriweather Regular |
| Flavor         | `body-italic/` | flavor text                   | MPlantin Italic         | Merriweather Italic  |
| Mana           | `mana/`        | mana and other card symbols   | Mana, already included  | Mana                 |
| Artist         | `type/`        | artist credit, in small caps  | Beleren Small Caps Bold | the Title default    |
| Collector info | `info/`        | collector number and set code | Gotham Medium           | the Rules default    |

The app keeps added fonts in a `fonts` folder under the per-OS user config
directory, one subfolder per role as named above, and Open folder in the same
setting shows it. Copying files into those subfolders by hand works too. Only
`.ttf` and `.otf` files are read, and only the first one in a folder is used.

Two other sources take priority over that folder. The `-fonts <dir>` flag points
the app at any folder laid out the same way, and a run from a checkout uses
`local-fonts/` as its [README](local-fonts/README.md) describes. Either one is
read-only from the app.

Layer compositing is handled by [impasto](https://github.com/odevine/impasto), a
pure-Go library for Photoshop-style layered images.

## Acknowledgements

Card data and art come from [Scryfall](https://scryfall.com). Please respect
their [API guidelines](https://scryfall.com/docs/api) when rendering at volume.
The app paces its own requests to their published rate limits.

mimic is unofficial fan content and is not affiliated with or endorsed by
Wizards of the Coast. Magic: The Gathering and its card names, text, and art
belong to their respective owners.
