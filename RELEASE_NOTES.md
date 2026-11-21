# tbible 2.1.0

v2.1 adds translation, Strong's, and passage-selection features while keeping
the single-binary, offline-first design.

## New features

- **Passage selection:** press `V`, move with the normal reader keys, press `V`
  again, then copy, bookmark, or annotate the whole range. Cross-chapter ranges
  are supported. The note editor keeps the selected passage visible while you
  write.
- **Strong's numbers:** press `s` in the reader or search preview to toggle
  inline tags. Press `K` on a tag to open its STEPBible dictionary entry in a
  side summary or full-screen overlay.
- **Downloadable translations:** ASV, WEB, YLT, and WMB are available as
  data-only `.tbp` packs. Press `T` to set a primary translation, add up to
  three parallel views, install or remove packs, or explicitly refresh the
  embedded catalog. Every visible translation is labelled.
- **Offline bundles:** each platform has an archive that includes the binary,
  catalog, and all four packs.
- **Heatmap everywhere:** `Ctrl-H` or `F2` opens the result heatmap even while
  typing in the omnibox.
- **Quick hide and ephemeral mode:** `Ctrl-X` blanks the screen instantly;
  `--ephemeral` or `TBIBLE_EPHEMERAL=1` keeps all user state in memory.

## Safety and privacy

There is no telemetry, no startup network access, and no background updates.
Downloads and catalog refreshes happen only on explicit user action and are
refused in ephemeral mode. Packs are verified with a catalog SHA-256 and
internal section hashes before atomic installation.

## KJV and data integrity

The KJV corpus format was updated to carry Strong's spans. The generator reports
348,884 tags across 31,099 verses and 19,570 lexicon headwords, with no missing
entries. The canonical counts remain 66 books, 1,189 chapters, and 31,102
verses.

Attribution for Scripture and lexicon sources is in `NOTICE.md`. Architecture,
standards, data sources, privacy, and release details are in `docs/`.

## Upgrading

No state migration is required. v2.0 settings continue to work; new settings
for translation, parallel views, and Strong's are additive. Existing `.tbp`
packs are not installed automatically.
