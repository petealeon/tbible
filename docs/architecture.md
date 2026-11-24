# Architecture

This document describes the v2.2 design. It is for contributors; the README is
the user-facing guide.

## Shape of the program

tbible is one static Go binary. There is no database server, runtime code
loading, or plugin system. All persisted data is either a small JSON state file
or a data-only translation pack.

```text
cmd/tbible      CLI and TUI entry point
cmd/tbgen       builds the embedded KJV corpus, index, and Strong's dictionary
cmd/tbpack      builds downloadable packs and the catalog from eBible USFM zips

internal/corpus immutable verse corpus reader/writer (TBC3)
internal/search immutable inverted index (TBI) and ranking
internal/usfm   USFM parsing and source normalisations
internal/strongs gzip JSON lexicon, lazy parse and lookup
internal/embedded go:embed blobs for KJV, KJV index, and Strong's
internal/packs  TBP1 container, verification, atomic install
internal/catalog embedded catalog plus explicit refresh/download
internal/library translation resolution and bounded LRU cache
internal/store  JSON state, settings, ephemeral mode
internal/tui    Bubble Tea model, screens, overlays, rendering helpers
internal/selfupdate explicit GitHub release check, verified download, binary swap
internal/version build-time version string
```

`internal/*` packages are deliberately not importable by outside modules.

## Corpus format: TBC3

The corpus is a memory-mappable byte slice, not JSON. `corpus.Write` emits:

```text
80-byte header
string blob
book table
chapter table
verse table        48 bytes per verse
inline span table
footnote table
heading table
Strong's table      8 bytes per Strong's span
```

The magic is `TBC3` and the format version is `2`. The version was bumped when
Strong's spans were added to the verse record. Every section offset and count
is bounds-checked on open; corrupted files fail closed.

`corpus.OpenBorrowed` views the embedded blob without copying it. `Prepare`
builds lookup maps and flattens decoration offsets once. `Verse.Strongs()`
returns a borrowed slice of `StrongSpan` values whose byte offsets point into
the shared string blob. Reader and writer live in the same package so offsets
cannot drift.

## Search index

`search.Build` creates an immutable index with token dictionary, postings,
document lengths, and per-book counts. `search.OpenBorrowed` also views bytes
without copying. `Index.CheckCorpus` verifies that the index was built from the
given corpus before either KJV or a pack is used for search. Ranking is BM25
with exact phrase, prefix, exclusion, scope, and reference parsing layered on
top in `internal/search` and `internal/ref`.

## USFM source handling

`internal/usfm` accepts eBible USFM zips. The KJV source must contain the 66
canonical books. Downloadable sources use two options:

- `CanonicalOnly()` skips extra files such as the Apocrypha in the WEB zip.
- `WebRomans()` moves WEB/WMB Romans 14:24–26 to 16:25–27 and drops a
  footnote-only placeholder at 16:25, keeping verses and decorations paired.

Verse numbers must be contiguous within chapters, and chapters sequential
within books. Inline `\w ...|strong="H7225"` tags become Strong's spans;
`\add`, `\f`, `\s`, and related markers become italics, footnotes, and
headings.

## Embedded data

`internal/embedded` exposes three `go:embed` blobs:

- `data/kjv.tbc` — the 31,102-verse KJV corpus with Strong's spans;
- `data/kjv.tbi` — its search index;
- `data/strongs.json.gz` — the STEPBible lexicon.

Each is parsed at most once behind a `sync.Once`. `StrongsBytes`,
`CorpusBytes`, and `IndexBytes` exist for verification tooling. The embedded KJV
is always available, even with no packs and no network.

## Strong's dictionary

`internal/strongs` stores a gzip-compressed JSON map from base codes (`H7225`,
`G3123`) to cleaned plain-text definitions. Parsing is lazy. `ParseTag` accepts
quoted and unquoted USFM attributes, extended codes such as `H7225G`, comma or
space separated lists, lowercase input, and folds extended forms back to the
base code for lookup. Definitions are capped at 8,000 bytes so a malformed
entry cannot flood the terminal.

## Packs: TBP1

A translation pack is one gzip-compressed file with extension `.tbp`:

```text
"TBP1\n"
uint32 manifest length + JSON manifest
uint32 corpus length   + TBC3 bytes
uint32 index length    + TBI bytes
```

The manifest records format, version, ID, name, source, license, section sizes,
and per-section SHA-256 hashes. The catalog records one SHA-256 over the whole
compressed file. `packs.VerifyAndInstall` checks the whole-file hash first,
then decodes and checks section hashes, then writes to a temporary file in the
destination directory and renames it atomically. A failed install never
replaces an existing pack.

Packs contain data only. There is no executable content, no hooks, and no code
loading path. IDs are lowercase alphanumeric with `-` and `_`; `packs.Path`
sanitises IDs before constructing a filename.

## Catalog and library

`internal/catalog` embeds `data/catalog.json` so the TUI can show available
translations offline. `Refresh` and `Download` are ordinary functions with
contexts, timeouts, and bounded bodies; the TUI calls them only from explicit
key actions. No network access occurs during startup, rendering, search, or
settings writes.

`internal/library` resolves the embedded KJV plus installed packs. It caches at
most four decoded corpora and indexes in an LRU, which is enough for one primary
and three parallel translations. `Invalidate` is called after install, remove,
or reinstall. `Available`, `Get`, `Name`, and `Has` are the only interfaces the
TUI needs.

## State and privacy

`internal/store` owns one JSON document with version 1. Corrupt or future files
are renamed to `.corrupt` rather than overwritten. `LoadEphemeral` creates a
store whose `Save` is a no-op; the TUI receives it through `tui.WithEphemeral`,
the `--ephemeral` flag, or `TBIBLE_EPHEMERAL=1`. Downloads and catalog refreshes
are refused in ephemeral mode.

`Ctrl-X` sets a `hidden` flag that makes `View` return an empty string. The next
key press clears the flag and is swallowed so it cannot trigger an action.

## TUI model

The Bubble Tea model has one `Update`/`View` pair and explicit screen state:

- search/omnibox with result sections and a preview;
- reader with sidebar and docked note editor;
- library with bookmarks, notes, history, saved searches, and calendar;
- overlays for help, command palette, Strong's entry, translations, and
  heatmap.

Overlays route keys before normal screens. `Ctrl-H`/`F2` is handled before
focus routing so the heatmap works while the omnibox owns `H`. `v` remains the
verse-number toggle, so passage selection is `V`. A finished `V` selection is
sticky for one action (`y`, `b`, or `n`) and is cleared by movement, book
changes, or an explicit `V`/`Esc`.

Parallel views are read-only `*corpus.Bible` values from the library cache.
The reader renders secondary translations under each primary verse with short
labelled prefixes; copy output uses the same labels. The note editor renders
the underlying `screenView` above itself so the passage being annotated stays
visible.

## Generation pipeline

`cmd/tbgen` parses the KJV USFM source, validates 66/1,189/31,102, writes
`kjv.tbc` and `kjv.tbi`, builds the Strong's JSON from TBESH/TBESG, and can
verify the embedded blobs against their recorded hashes.

`cmd/tbpack` parses each public-domain eBible zip, applies `CanonicalOnly` and
`WebRomans` where needed, writes a `.tbp`, verifies it by decoding it again and
checking the index against the corpus, then writes a deterministic catalog with
URLs, sizes, dates, and SHA-256 hashes. The generated catalog is committed so
release hashes and embedded hashes cannot drift.
