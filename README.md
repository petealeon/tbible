# tbible — Terminal Bible (KJV)

A fast, keyboard-driven KJV Bible browser and search tool that runs in your
terminal. Built on [fzf](https://github.com/junegunn/fzf) and SQLite (FTS5).

<!-- TODO: add a screenshot at docs/screenshot.png -->

## Features

- **Browse or jump straight to a reference** — type `john 3`, `john 3:16`, or a
  book name (e.g. `psalms`), fzf reloads the list as you type.
- **Full-text search** with prefix matching, plus an OT/NT result breakdown.
- **Chapter and verse previews** that update as you move through the list.
- **Copy anything** — a single verse or a whole chapter — straight to the
  clipboard (Wayland and X11), with an optional desktop notification.
- **Search highlighting** in the chapter preview while you type.
- **Keyboard-first** — no mouse needed. See keybindings below.

## Requirements

- `bash` 4+
- `sqlite3` (with FTS5 support — stock on virtually all distros)
- `fzf` 0.4x+
- Python 3 (only needed to build the database from source)

Optional:

- `wl-clipboard` (`wl-copy`), `xclip`, or `xsel` — for copy-to-clipboard
- `libnotify` (`notify-send`) — for copy notifications

## Install

```sh
git clone https://github.com/USER/terminal-bible.git
cd terminal-bible
./install.sh             # installs tbible + builds the database
./install.sh --desktop   # also installs an app-menu entry + icon
```

The installer puts `tbible` in `~/.local/bin/` and, on first run, builds a
SQLite database from `data/kjv.json` at
`~/.local/share/terminal-bible/bible.db`.

## Usage

```sh
tbible
```

Type to filter. Use `Tab` in the main list to copy a chapter, or `Enter` to
drill into a chapter and pick a verse. Press `Alt-s` to launch a global verse
search over the whole Bible.

### Keybindings

| Key                    | Action                        |
|------------------------|-------------------------------|
| `enter`                | open chapter → verse list     |
| `enter` / `tab`        | copy verse                    |
| `tab`                  | copy chapter                  |
| `alt-s`                | global verse search           |
| `alt-p`                | toggle preview                |
| `alt-j` / `alt-k`      | scroll preview                |
| `esc`                  | back                          |

### Examples

```
john 3:16        # exact verse
romans 8         # whole chapter
2 corinthians 5  # second-level book chapter
shepherd         # keyword search (shows OT/NT counts)
```

### Environment variables

- `DB_FILE` — path to the SQLite database (default:
  `~/.local/share/terminal-bible/bible.db`)
- `TRANSLATION` — label used in copies and previews (default: `KJV`)

## The data

The KJV text in `data/kjv.json` is in the **public domain in the United
States** — you can read, use, and redistribute it freely. (Note: the King James
Version has a Crown-patent restriction in the United Kingdom; check local law
if you distribute it there.) 66 books, 31,100 verses, with the OT/NT split used
by the search stats.

The database is generated with `scripts/build_db.py`:

```sh
python3 scripts/build_db.py            # uses defaults
python3 scripts/build_db.py --db /tmp/test.db
```

## Project layout

```
tbible                  main script (single dependency-free binary)
install.sh              installer
scripts/build_db.py     kjv.json → SQLite DB + FTS5 index
data/kjv.json           source text (public domain)
packaging/              desktop entry + icon
```

## License

Code: MIT (see `LICENSE`). Bible text: public domain (KJV).