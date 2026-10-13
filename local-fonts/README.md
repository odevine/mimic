# Local font overrides

Drop a font here to render a role with it instead of the engine's built-in
font. The dropped files are gitignored, and this README is the only tracked
file.

`rendercard` uses this directory automatically when it exists, so no `-fonts`
flag is needed for a repo-root or `engine/` run. Pass `-fonts <dir>` to point
somewhere else.

## How it works

Each role has its own folder. Drop a font into the folder for its role and the
first `.ttf` or `.otf` in that folder is used, so the file keeps whatever name
it came with. Nothing needs renaming, and a file left loose at the top level is
ignored.

The [top-level README](../README.md#templates-and-fonts) lists every role
folder, what it draws, and its built-in font.

Rules and flavor share no folder: keep an italic face in `body-italic/` and a
roman one in `body/`.

A file that fails to load leaves its role on the default, so a render that
looks unchanged after dropping a font usually means the file was not a valid
`.ttf` or `.otf`.
