# Data sources

All bundled and downloadable text must be redistributable as data. This file
records what is used and why.

## KJV

- Source: eBible.org `eng-kjv2006` USFM zip.
- URL: <https://ebible.org/Scriptures/eng-kjv2006_usfm.zip>
- Contents used: 66 canonical books, 1,189 chapters, 31,102 verses, KJV
  marginal footnotes, italics, headings, red-letter spans, and inline Strong's
  numbers.
- Verified SHA-256 of the source USFM zip:
  `b6faa67117ccaba49df9e7df224a553f11758ae7c5d58f0f7979e2b73ac4480b`.
- Notice: public domain outside the UK; Crown copyright applies in the UK.

The canonical counts and the corpus hash are checked by `cmd/tbgen --verify`.

## Strong's lexicon

- Sources: STEPBible TBESH (Hebrew) and TBESG (Greek) text files from
  <https://github.com/STEPBible/STEPBible-Data/tree/master/Lexicons>.
- License: CC BY 4.0. Attribution is required and is provided in `NOTICE.md`
  and the in-app help.
- Generated file: `internal/embedded/data/strongs.json.gz`.
- Coverage: 19,570 headwords after normalising leading zeroes. The generated
  KJV corpus contains 348,884 Strong's tags across 31,099 verses, and every tag
  resolves to a dictionary entry.
- The builder converts source HTML to plain text and removes tags that would
  break terminal rendering. Definitions longer than 8,000 bytes are truncated
  at generation time and again defensively at lookup.

## Downloadable translations

All packs come from public-domain eBible.org USFM archives and contain text
plus an index only.

| ID | eBible ID | Name | Source note |
| --- | --- | --- | --- |
| `asv` | `eng-asv` | American Standard Version | Public domain, 1901 |
| `web` | `eng-web` | World English Bible | Public domain; eBible trademark |
| `ylt` | `engylt` | Young's Literal Translation | Public domain, 1898 |
| `wmb` | `engwmb` | World Messianic Bible | Public domain |

Source URL pattern: `https://ebible.org/Scriptures/<id>_usfm.zip`.

`cmd/tbpack` applies `usfm.CanonicalOnly()` to all sources because some eBible
archives include the Apocrypha (the WEB zip contains 83 USFM files). It applies
`usfm.WebRomans()` to WEB and WMB because those editions place Romans
14:24–26 where other editions have 16:25–27 and include a footnote-only verse
at 16:25. The normalisation moves the three verses and drops the placeholder so
all packs share the canonical 16-chapter Romans structure.

Every generated pack is decoded again, checked with `Index.CheckCorpus`, and
its SHA-256 is written to the embedded catalog. Pack generation is expected to
be deterministic for a fixed source and date.

## Deliberately excluded

- Translations with a separate EULA or ongoing licence fee.
- Study notes, commentaries, and cross-reference databases with restrictive
  licences.
- Any source that requires runtime attribution UI beyond a static notice.
- Unofficial files whose redistribution status cannot be established from the
  publisher's own copyright page.

When in doubt, leave a translation out. The catalog is small and adding a
public-domain pack later does not change the binary format.
