# Engineering standards

These rules keep tbible small, auditable, and safe to run in sensitive
environments.

## Go and build

- Target Go 1.27 or newer. Keep `go.mod` tidy.
- `CGO_ENABLED=0` for all release builds; the result must be a single static
  executable per platform.
- Run `gofmt`, `go vet ./...`, `go test ./...`, and `go build ./...` before any
  commit. Release builds additionally run `make dist VERSION=<version>` and
  verify the checksum manifest.
- Keep platform differences at the edges: clipboard helpers and path lookup in
  their own files/packages. The core corpus, search, and TUI logic is portable.
- No runtime code generation, plugin loading, or `exec` of bundled data.

## Privacy and network

- No telemetry, analytics, crash reporting, update checks, or network traffic
  on startup, search, render, or save.
- Network access is allowed only inside an explicit user action: catalog
  refresh, pack install, `tbible --check-update`, or `tbible --update`. Every
  request uses a context and a timeout and bounds the response body.
- Ephemeral mode must write no state and must refuse network actions. Never
  cache downloaded bytes to disk in that mode.
- Do not add instructions for bypassing corporate proxies, network filters, or
  local security policy. The offline bundles are the supported answer for
  restricted machines.
- Do not log verse text, notes, search queries, or paths in release builds.

## Data integrity

- Binary formats start with a magic value and version. Readers check every
  offset, count, and section length before using it.
- Generated data is verified at build time: corpus/index consistency, canonical
  book counts, Strong's headword coverage, and pack section hashes.
- Downloads are verified against a catalog SHA-256 before installation;
  internal section hashes are checked after decoding. Installs write to a
  temporary file and rename atomically.
- User state is never silently overwritten. Unreadable files are renamed with a
  `.corrupt` suffix and reported in the status line.
- Strong's IDs are folded to base codes for lookup; definitions are sanitised
  from HTML and capped in length.

## Package boundaries

- `internal/corpus` owns the corpus format. Reader and writer must change
  together and the format version must be bumped for incompatible changes.
- `internal/search` owns ranking and index format. An index must be checked
  against the corpus it was built for.
- `internal/packs` owns pack encoding and install safety. It never interprets
  verse text.
- `internal/catalog` owns the embedded catalog and explicit network helpers.
- `internal/library` is the only place that chooses between embedded KJV and
  installed packs for the app and CLI.
- `internal/store` is the only writer of user state.
- `internal/tui` owns rendering and key routing. It must not perform blocking
  network work in `Update`; use a `tea.Cmd` that returns a typed message.

## Keyboard conventions

- Every feature remains keyboard-only. New actions need a key, a help entry,
  and a command-palette entry where appropriate.
- `v` is permanently the verse-number toggle. Passage selection is `V`.
- `H` is text while the omnibox is focused; heatmap is `Ctrl-H` or `F2`.
- `Ctrl-X` blanks the screen and the restoring key is swallowed.
- Overlays consume keys before screens. Escape leaves the topmost overlay
  first, then cancels a selection, then returns home.
- A finished `V` range is consumed by the next `y`, `b`, or `n`, then cleared.
  Movement or navigation clears stale ranges.

## Documentation and user interface

- README leads with benefits and features. Keep the measured speed figures.
  Release mechanics and internal debates belong in `docs/`, not the README.
- Keep per-OS install instructions separate and explicit.
- Help text and command palette must match the implemented key map.
- Text notices and licenses live in `NOTICE.md`; user-facing summaries live in
  the README. Never remove required attribution.
- Screenshots are regenerated when the layout changes materially.

## Tests

- New binary formats need round-trip, truncation, bad-hash, and wrong-version
  tests.
- Network helpers need local `httptest` coverage for success and failure.
- TUI state machines need tests for the exact key sequence that triggers them,
  especially overlays, selection lifecycle, and ephemeral guards.
- Generation changes need canonical count checks: 66 books, 1,189 chapters,
  31,102 verses for KJV and every pack.
