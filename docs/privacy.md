# Privacy model

tbible is designed for people who may not want anyone to know that they are
reading the Bible, including on a shared or monitored computer.

## What the program never does

- It never sends telemetry, analytics, search terms, notes, bookmarks, or
  installation status anywhere.
- It never contacts a server at startup, during search, while rendering, or
  when saving state.
- It never checks for updates in the background.
- It never loads remote content into the terminal.

## What requires a deliberate action

Only two actions use the network:

1. `T` → `U` refreshes the translation catalog.
2. `T` → `Enter` on a catalog entry downloads and verifies a pack.

Both are foreground, user-initiated commands with visible status text. They are
refused when ephemeral mode is active. The embedded KJV, Strong's dictionary,
search, notes, and heatmap work with networking completely unavailable.

## Local state

By default, user data is one JSON file in the OS configuration directory and
packs are files in a `packs` subdirectory. Paths can be moved with
`TBIBLE_STATE_DIR` and `TBIBLE_PACKS_DIR`, including to removable media.

Start with `tbible --ephemeral` or set `TBIBLE_EPHEMERAL=1` to keep notes,
bookmarks, history, and settings in memory only. In that mode `Save` is a
no-op, downloads are disabled, and closing the terminal leaves no user state
behind. The embedded KJV remains available.

## Quick hide

`Ctrl-X` blanks the terminal completely. The next key restores the previous
screen and is swallowed, so it cannot be interpreted as a search, copy, or
navigation command. Quick hide does not write anything and does not alter
state.
