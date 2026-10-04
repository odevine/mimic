# Local font overrides

Drop the real card fonts here to render with them instead of the embedded
open-license defaults. The dropped files are gitignored, since the real fonts
are copyrighted and not ours to redistribute. This README is the only tracked
file.

`rendercard` uses this directory automatically when it exists, so no `-fonts`
flag is needed for a repo-root or `engine/` run. Pass `-fonts <dir>` to point
somewhere else.

## How it works

Each role has its own folder. Drop a font into the folder for its role and the
first `.ttf` or `.otf` in that folder is used, so the file keeps whatever name
it came with. Nothing needs renaming, and a file left loose at the top level is
ignored.

| folder         | draws                          | real cards use          | default when empty   |
|----------------|--------------------------------|-------------------------|----------------------|
| `title/`       | card name, type line, P/T      | Beleren Bold            | Big Shoulders Bold   |
| `body/`        | rules text, copyright line     | MPlantin                | Merriweather Regular |
| `body-italic/` | flavor text                    | MPlantin Italic         | Merriweather Italic  |
| `mana/`        | mana and other card symbols    | Mana (already embedded) | Mana                 |
| `type/`        | artist credit, in small caps   | Beleren Small Caps Bold | the `title/` default |
| `info/`        | collector number and set code  | Gotham Medium           | the `body/` default  |

The `mana/` role already resolves to an embedded Mana font, so drop a file there
only to override it. Rules and flavor share no folder: keep the italic Plantin
in `body-italic/` and the roman one in `body/`.

A file that fails to load leaves its role on the default, so a render that
looks unchanged after dropping a font usually means the file was not a valid
`.ttf` or `.otf`.
