# mpcfill

`mpcfill` lays rendered cards out as an [MPC Autofill](https://github.com/chilli-axe/mpc-autofill)
project: a directory of PNGs plus the `cards.xml` order file MPC Autofill uses to
place an order with MakePlayingCards. It decides every file name and writes the
order file, but renders nothing itself, and depends only on the standard library.

```go
import "github.com/odevine/mimic/engine/mpcfill"
```

## The model

```go
type Face struct {
    Name string
}

type Card struct {
    Front    Face
    Back     *Face  // nil: the project cardback. Set: a double-faced card
    Quantity int    // copies; non-positive counts as one
}

type Project struct {
    Stock    Stock  // empty is S30
    Foil     bool
    Cardback *Face  // shared by every card without its own Back
    Cards    []Card
}

layout, err := mpcfill.Plan(project)
// render layout.Cards[i].Front, .Back, and layout.Cardback to PNGs under dir
err = mpcfill.WriteOrder(dir, layout)
```

Writing a project takes two steps with the rendering in between. `Plan`
validates the project and gives every face a file, a slash-separated path
relative to the project directory. `layout.Cards` lines up with
`project.Cards`. The caller renders each face to its path, however it likes:
in parallel, with retries, with progress. `WriteOrder` then writes `cards.xml`,
but only once every planned image is on disk and not empty, so a project with
a failed card never gets an order file that points at nothing.

Render each face once. A face used for 60 copies of a card is still one file:
set `Quantity` rather than repeating the card.

Every card shares the project's `Cardback` unless it has a `Back` of its own,
which is how double-faced cards are expressed. `Cardback` may be nil only when
every card is double-faced.

## What gets written

```
dir/cards.xml
dir/fronts/<name>.png     card fronts
dir/backs/<name>.png      backs of double-faced cards
dir/cardback/<name>.png   the shared cardback
```

Cards take consecutive slots in slice order, so a project of `{A ×3, B, C ×2}`
puts A in slots 0 to 2, B in 3, and C in 4 and 5. The order file refers to every
image by its path relative to `dir`, for example `./fronts/Island.png`.

`Plan` touches no files, so an invalid project is caught before anything is
rendered. Write into an empty directory, because the website indexes every
image it finds there.

## Using the output

**Desktop tool.** Point it at the directory with its `-d dir` flag, or run it
from inside `dir`. It changes into that directory, finds `cards.xml`, and
resolves each image path relative to it.

**Website.** Open *Sources*, choose `dir` under *Local Folder*, then import
`cards.xml` with the XML importer, under *Add Cards → XML*, or the *XML* panel
an empty project shows. The website only knows the images in a
connected folder, so the folder has to be connected before the import. The
website also refuses an import that would take the project past 612 cards, so
import into an empty project.

## Why names are rewritten

`Face.Name` becomes the file name, but not verbatim. It keeps ASCII letters,
digits, hyphens, and apostrophes. Accented Latin letters are written in plain
ASCII first. Everything else, including any other non-ASCII letter, becomes a
single space:

| Name                         | File                             |
| ---------------------------- | -------------------------------- |
| `Lightning Bolt`             | `Lightning Bolt.png`             |
| `Fire // Ice`                | `Fire Ice.png`                   |
| `Island (Unsanctioned)`      | `Island Unsanctioned.png`        |
| `Æther Vial`                 | `AEther Vial.png`                |
| `Séance`                     | `Seance.png`                     |
| `Mishra’s Factory`           | `Mishra's Factory.png`           |
| `123`, `!!!`, `日本語`       | `Card.png`                       |

Names that collide once rewritten, compared case-insensitively, get a numeric
suffix: `Goblin.png`, `goblin 2.png`, `GOBLIN 3.png`. So do Windows device
names such as `Con` and `COM1`, which Windows refuses as file names. Names are
capped at 100 characters.

The reason is how the website imports an order. For each card it reads the
image id, but also the card's search query, and once the import finishes it
runs that query. **If the imported image is not among the query's results, the
website swaps in the first result** and lists the card under *Invalid
Identifiers*. If the query finds nothing at all, the image is dropped without
even that. So a card only survives the import if searching for its own query
finds its own file.

The website's search normalizes both sides: it lowercases, drops digits and
punctuation, deletes anything in brackets, and reads `(...)` in a query as a
set code. A name containing any of that can produce a query that
no longer finds its file. `mpcfill` sidesteps all of it by writing only
characters that survive normalization unchanged, then using the lowercased file
name as the query. The two always match.

ASCII in particular is about identity, not search. The order file has to name
each image byte for byte, and a non-ASCII file name can come back from the
filesystem in a different Unicode normalization than the one written.

## Why the fixed folders

The website decides whether an image is a card or a cardback from the name of
the folder that directly holds it: a folder whose name contains "cardback" holds
cardbacks, one containing "token" holds tokens. Two things follow.

The cardback has to live in a folder named for it, or the website will not
offer it as a cardback. And fronts cannot sit in the project root, because the
root's name is whatever the caller chose. Writing fronts to the root of a
directory called `goblin tokens` would make the website retype every card.

The folder names avoid everything else the website strips or reads from folder
names: `{EN} ` language prefixes, `[tag]` and `(tag)` groups, and a leading `!`,
which hides a folder from the index entirely.

## Limits

| Rule                                     | Error             |
| ---------------------------------------- | ----------------- |
| At least one card                        | `ErrNoCards`      |
| At most `MaxProjectSize` (612) cards     | `ErrTooManyCards` |
| `Stock` is one of the five constants     | `ErrStock`        |
| No foil on `P10`                         | `ErrFoil`         |
| A cardback unless every card has a back  | `ErrNoCardback`   |
| Every planned image exists, `WriteOrder` | `ErrMissingImage` |

612 is MPC Autofill's own ceiling. The website cannot import a larger order, and
the desktop tool only splits orders when it combines several order files. For a
bigger order, split the cards across several projects, each in its own
directory.

`Stock` values are MakePlayingCards' exact names, such as `(S30) Standard Smooth`,
because both tools compare the string exactly.

## Image size

`mpcfill` never looks inside an image, so size is up to the caller. MPC
Autofill measures resolution by height, treating a 1110-pixel-tall card as 300
DPI. A full-bleed card at 300 DPI is about 816×1111 pixels: the 63×88 mm card
plus 0.12 inches (3.048 mm) of bleed on each side.

The website hides images above 1500 DPI (5568 pixels tall) or 30 MB with its
default search settings, and an image it hides cannot survive an import. `DPI`,
`MaxDPI`, and `MaxImageBytes` state those rules for checking an image before it
goes in a project. Mimic's templates top out at 1200 DPI, and a card rendered
at that resolution is around 10 MB.

## What is not here

- **Images hosted elsewhere.** Every image is a local file the caller writes.
  Google Drive ids, which the format also allows, are not supported.
- **Tokens.** Tokens print like any other card, so they go in `Cards`. The
  website will list them as cards, not tokens.
- **Splitting large orders.** Past 612 cards, `Plan` returns an error rather
  than choosing split points, since where to split affects MakePlayingCards'
  pricing brackets.
