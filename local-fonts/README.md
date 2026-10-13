# Local font pool

Drop any `.ttf` or `.otf` here to make it available to `rendercard`. The dropped
files are gitignored, and this README is the only tracked file.

`rendercard` uses this directory automatically when it exists, so no `-fonts`
flag is needed for a repo-root or `engine/` run. Pass `-fonts <dir>` to point
somewhere else.

## How it works

The folder is a pool of fonts, found by the family and style names inside each
file. File names and subfolders mean nothing, so a font can sit anywhere under
the folder and keep whatever name it came with. A font is named as its family
and style joined by a space, such as `Gotham Medium` or `Plantin MT Pro Italic`,
and the capitalization does not matter.

A role draws with its own font until a font from the pool is assigned to it.
Assign one with `-font role="Family Style"`, repeating the flag for more roles:

```
go run ./cmd/rendercard -name "Lightning Bolt" -font title="Gotham Medium" -font body="Plantin MT Pro Italic"
```

A role is a standard one, `title`, `body`, `body-italic`, `mana`, `small-caps` or
`info`, or one the template defines. Any font in the pool can be assigned to any
role. The [top-level README](../README.md#templates-and-fonts) lists the standard
roles and the built-in font each one draws with.

No font has every letter. A letter the font assigned to a role lacks is drawn from
the role's own font, and then from the built-in body font, at the same size on the
same baseline. A line is as tall as the assigned font makes it.

`rendercard` prints what it found in the pool and warns about a name it cannot
find. A file that is not a valid font, or that has the same family and style as an
earlier one, is skipped with a note, so a role that looks unchanged after an
assignment usually names a font that is not in the pool.
