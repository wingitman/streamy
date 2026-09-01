# streamy Development Rules

Read `docs/blueprint.md` and `docs/checklist.md` before changing the TUI.
Preserve measured responsive layout, variable-height scrolling, contextual
hints, shared TOML themes, layout-derived mouse hitboxes, and asynchronous
external commands. Run `make test` and `make build-all` after meaningful changes.

The baseline uses Bubble Tea v2, Lip Gloss v2, and display-cell measurement.
Do not reintroduce byte-based wrapping, fixed mouse coordinates, synchronous
clipboard/editor/update work, or placeholder history and rollback behavior.
