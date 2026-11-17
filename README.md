# tbible — Terminal Bible (KJV)

![tbible v2 search and reader views](screenshots/tbible-v2.png)

A fast, keyboard-driven King James Bible for the terminal. The complete KJV
text and a prebuilt full-text search index are compiled into one static binary,
so there is no database to build, no service to run, and no network dependency.

v2 is the current release. It is a rewrite of the original bash/fzf/SQLite
app in Go with Bubble Tea. Windows 11 + Windows Terminal are the primary
target; Linux and macOS are fully supported. The legacy v1 application is
preserved in [`legacy/v1/`](legacy/v1/README.md) and on the `v1` branch.

## What's new in v2

- **One static executable.** Corpus, search index, styles, headings, and KJV
  marginal notes are embedded with `go:embed`. v1 needed bash, fzf, sqlite3,
  Python, and a generated database.
- **Much faster.** Cold start plus the first search is roughly 20 ms; repeated
  searches take tens of microseconds. v1 paid shell and SQLite FTS overhead on
  every invocation.
- **Lower memory at rest.** Borrowed embedded views, compact varint postings,
  and eager validation keep the full corpus resident without high long-running
  RSS. The TUI defaults to `GOGC=50`, overridable through `GOGC`.
- **Verified, corrected text.** 66 books, 1,189 chapters, and 31,102 verses
  from eBible.org's `eng-kjv2006` USFM edition. The generator checks canonical
  counts and known verses that the old corpus had dropped or shifted.
- **One omnibox.** References, ranges, phrases, prefixes, exclusions, and
  keywords share a single input. There are no hidden search modes to learn.
- **Richer reading.** Verse numbers, paragraphs, poetry, section headings,
  psalm titles, supplied-word italics, red-letter text, KJV marginal notes,
  and your own notes inline with a distinct `✎` marker.
- **Result heatmap.** See keyword concentration across all 66 books, drill into
  chapters, and toggle density vs raw counts. Wide terminals also show a live
  compact heatmap beside the verse preview.
- **Calendar view.** Browse notes, bookmarks, and reading history by local
  date in a Sunday-first heatmap with per-day entries and inline filtering.
- **Persistent local user data.** Bookmarks, notes, reading history, and
  scoped saved searches live in one versioned JSON file. Corrupt or
  future-version files are backed up rather than silently overwritten.
- **Scriptable CLI.** `--read`, `--search`, `--plain`, `--limit`, `--scope`,
  `--book`, and `--json` make the same binary usable from scripts.
- **Cross-platform binaries.** Linux, macOS, and Windows builds are static and
  need no C compiler at runtime or build time.

## Quick start

### Install a prebuilt binary (recommended)

Prebuilt archives are published as assets on the `v2.0.0` GitHub Release. The
installers verify the download against the published SHA-256 manifest.

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/petealeon/tbible/main/install.sh | bash
```

With options:

```sh
curl -fsSL -o install.sh \
  https://raw.githubusercontent.com/petealeon/tbible/main/install.sh
chmod +x install.sh
./install.sh --version 2.0.0 --bin-dir "$HOME/.local/bin" --desktop
```

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -Command ^
  "irm https://raw.githubusercontent.com/petealeon/tbible/main/scripts/install.ps1 | iex"
```

Or from a checkout:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install.ps1
```

Use `-AddToPath` to add the install directory to your user `PATH`. Both
installers accept a mirror override: `TBIBLE_RELEASE_BASE` for `install.sh` and
`-BaseUri` for `install.ps1`.

> Until the `v2.0.0` release assets are uploaded, use the source-build path
> below. Release archives are produced locally by `make dist`.

### Build from source

Requires Go 1.27 or newer (the version pinned in `go.mod`). No C compiler is
needed; release builds set `CGO_ENABLED=0`.

From a checkout:

```sh
git clone https://github.com/petealeon/tbible.git
cd tbible

./install.sh --source          # build and install to ~/.local/bin
make build                     # or just build ./bin/tbible
./bin/tbible --version
```

On Windows:

```powershell
git clone https://github.com/petealeon/tbible.git
cd tbible
powershell -ExecutionPolicy Bypass -File .\scripts\install.ps1 -Source
.\bin\tbible.exe --version
```

The generated data files under `internal/embedded/data/` are committed, so a
normal build needs no network access and no generation step.

## Dependencies

| Platform | To run | To build from source | Clipboard `y` |
| --- | --- | --- | --- |
| Linux | UTF-8 terminal (Windows Terminal, Alacritty, kitty, etc.) | Go 1.27+, `git`, optional `make`/`bash` | `wl-copy`, `xclip`, or `xsel` |
| macOS | Terminal.app, iTerm2, or Windows Terminal | Go 1.27+, `git`; Xcode Command Line Tools supply `git`/`make` | `pbcopy` (built in) |
| Windows | Windows 11 + Windows Terminal recommended | Go 1.27+ and PowerShell 5.1+; Git for Windows optional | `clip.exe` (built in) |

Go can be installed from <https://go.dev/dl/>. On macOS, `brew install go` is
also fine. Python 3 is only needed for `make dist` and the legacy v1 database
builder, not for normal use or building.

Notes:

- Clipboard helpers are optional. Copy failures are shown in the status line;
  reading, searching, and navigation work without them.
- macOS may quarantine downloaded binaries. If macOS blocks first launch, run
  `xattr -d com.apple.quarantine /path/to/tbible`, or build from source.
- Windows SmartScreen may warn on an unsigned first release. Check the
  published checksum, then choose **More info → Run anyway**.

### Should releases include binary downloads?

Yes. The whole design is a single static executable: shipping prebuilt binaries
is the lowest-friction path for users and avoids making them install Go just to
read the Bible. Keep source builds available for developers and for platforms
without a published archive. Always publish a checksum manifest so the
installers and cautious users can verify downloads.

`make dist` builds and packages:

```text
tbible-2.0.0-linux-amd64.tar.gz
tbible-2.0.0-linux-arm64.tar.gz
tbible-2.0.0-darwin-amd64.tar.gz
tbible-2.0.0-darwin-arm64.tar.gz
tbible-2.0.0-windows-amd64.zip
tbible-2.0.0-checksums.txt
```

## Verification

```sh
gofmt -l .
make test
make vet
make race
go run ./cmd/tbgen --verify
make build windows
./bin/tbible --version
```

Tests cover reference/range parsing, corpus validation and corruption,
tokenization and query matching, index round-trips, state recovery, scoped
saved searches, CLI helpers, date aggregation, and the TUI flows. Fuzz targets
guard the binary readers and query parser against malformed input.

## Using the TUI

Run `tbible` with no arguments. The omnibox is focused at launch.

```text
John 3:16          single reference
shepherd           keyword, ranked by BM25
"still waters"     exact phrase
love -world        require love, exclude world
shep*              prefix
1 Cor 13:4-7       chapter/verse range
John 3-5           chapter range
John 3:16-5:2      cross-chapter range
john 3:16; rom 5:8 several references
3:16               relative to the current book
```

The visible scope chip cycles with `f`: all books, Old Testament, New
Testament, or the current book. `Ctrl-S` saves the query and its scope. Press
`H` for the full result heatmap; in terminals at least 120×30 a compact live
heatmap also sits below the selected verse preview.

![tbible v2 result heatmap showing match concentration by book](screenshots/tbible-v2-heatmap.png)

The Library (`L`) holds bookmarks, notes, history, saved searches, and the
Calendar tab. In Calendar, `h`/`l` move by day, `[`/`]` by month, `1`–`4` or
`s` switch source, `f` filters entries, and `Enter` opens the selected verse.
Press `?` at any time for the complete key list.

Common keys:

| Key | Action |
| --- | --- |
| `/` | Focus the omnibox |
| `↑` `↓` | Move selection |
| `Enter` | Open the selected reference or verse |
| `Tab` | Next result section / Calendar focus |
| `f` | Cycle scope, or open Calendar filter |
| `H` | Result heatmap |
| `L` | Library |
| `Ctrl-P` or `:` | Command palette |
| `y`, `b`, `n` | Copy, bookmark, or note the focused verse/range |
| `i` | Toggle KJV marginal notes and your inline notes |
| `Esc` | Clear query, leave reader, or go back |
| `Ctrl-C` | Quit |

## Command line

```sh
tbible --read "John 3:16"
tbible --read "Psalms 23" --plain
tbible --read "John 3:16-5:2" --json
tbible --search "John 3:16; Romans 5:8" --json
tbible --search shepherd --limit 10
tbible --search faith --scope nt
tbible --search grace --book Romans
tbible shepherd                       # shorthand for --search
tbible --version
```

`--read` prints every referenced verse, deduplicating overlaps and splitting
cross-chapter ranges at chapter boundaries. `--plain` removes headings and
verse numbers; `--json` emits structured output for both read and search.

## User data

State is stored at:

- Windows: `%AppData%\tbible\state.json`
- macOS: `~/Library/Application Support/tbible/state.json`
- Linux: `$XDG_CONFIG_HOME/tbible/state.json` or
  `~/.config/tbible/state.json`

Set `TBIBLE_STATE_DIR` to override the directory. Writes are versioned,
atomic, and debounced; history is capped at the newest 100 entries. If a file
is unreadable or from a future version, it is renamed to `state.json.corrupt`
(or `.corrupt.1`, `.corrupt.2`, …) and the app starts with a visible warning.

v1 did not persist user notes, bookmarks, or history, so there is no state to
migrate. Existing v1 users can keep using the legacy app or start fresh in v2.

## Data provenance

The corpus is generated deterministically from eBible.org's `eng-kjv2006` USFM
edition, vendored at `data/eng-kjv2006_usfm.zip`:

```text
SHA-256 b6faa67117ccaba49df9e7df224a553f11758ae7c5d58f0f7979e2b73ac4480b
```

| Generated file | Purpose |
| --- | --- |
| `internal/embedded/data/kjv.tbc` | binary corpus with text, styles, notes, headings |
| `internal/embedded/data/kjv.tbi` | BM25 inverted index |
| `legacy/v1/data/kjv.json` | clean JSON source for the legacy v1 database builder |

Regenerate or verify with:

```sh
go run ./cmd/tbgen --verify   # validate only
go run ./cmd/tbgen            # rewrite corpus, index, and legacy JSON
```

Binary formats are versioned (`TBC2`, `TBI2`) and reject unknown versions.

## Repository layout

```text
cmd/tbible              CLI and TUI entry point
cmd/tbgen               deterministic corpus/index/JSON generator
internal/activity       date grouping for the calendar
internal/corpus         Bible model and binary corpus format
internal/usfm           USFM parser
internal/search         tokenizer, BM25 index, query parser, scopes
internal/ref            reference parser and book aliases
internal/store          versioned JSON user state
internal/tui            Bubble Tea screens and rendering
internal/embedded       the only place generated data is wired in
data/                   vendored USFM source
legacy/v1/              preserved bash/fzf/SQLite application
install.sh              Linux/macOS installer
scripts/install.ps1     Windows installer
scripts/dist.sh         release archive builder
packaging/              desktop entry
```

v2 ships the KJV core only. The embedded-data seam is deliberately narrow so
optional packs (commentaries, dictionaries, additional translations) can be
added later without changing the reader, store, or query syntax.

## Legacy v1

The original bash/fzf/SQLite app is still available and supported:

- source: [`legacy/v1/`](legacy/v1/README.md)
- branch: `v1`
- installer: `legacy/v1/install.sh`

It needs bash 4+, fzf 0.60+, sqlite3 with FTS5, and Python 3 to build its
database. New development happens in v2.

## License and textual notice

This project is licensed under the terms in [LICENSE](LICENSE).

The King James Version is in the public domain in the United States and most
of the world outside the United Kingdom. In the UK the Authorized Version is
subject to Crown copyright; this project does not grant any rights to the KJV
text and is not affiliated with the Crown. The text used here is eBible.org's
`eng-kjv2006` edition, which states that it is public domain outside the UK.
