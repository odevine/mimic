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

| folder        | role                       | status         |
|---------------|----------------------------|----------------|
| `title/`      | card name, type line, P/T  | used           |
| `body/`       | rules text                 | used           |
| `body-italic/`| flavor text                | used           |
| `mana/`       | mana symbols               | used           |
| `type/`       | small-caps type line       | not wired yet  |
| `info/`       | collector, set, copyright  | not wired yet  |
| `glyphs/`     | non-mana glyphs            | not wired yet  |

The `mana/` role already resolves to an embedded Mana font, so drop a file there
only to override it. Rules and flavor share no folder: keep the italic Plantin
in `body-italic/` and the roman one in `body/`.

The bottom three folders hold fonts the engine does not render yet. The type
line currently draws with the `title/` font, and the collector, set, and
copyright lines and non-mana glyphs are not drawn at all, so files in `type/`,
`info/`, and `glyphs/` sit staged until those roles are wired.
