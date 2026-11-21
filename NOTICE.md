# Notices and attributions

tbible includes public-domain Scripture texts and a Creative Commons lexicon.
This file records their sources so they can be checked and attributed.

## King James Version

- Text: eBible.org `eng-kjv2006` USFM edition.
- Source: <https://ebible.org/eng-kjv/>
- Status: public domain outside the United Kingdom.
- Note: in the UK the Authorized Version is subject to Crown copyright. No
  rights to the KJV text are granted by this project, which is not affiliated
  with the Crown.
- Strong's numbers embedded in the KJV corpus come from the same eBible source.

## Strong's dictionary

- Work: Translators' Brief lexicon of Extended Strong's for Hebrew (TBESH) and
  Greek (TBESG).
- Publisher: STEPBible.org.
- Source: <https://github.com/STEPBible/STEPBible-Data/tree/master/Lexicons>
- License: [Creative Commons Attribution 4.0 International](https://creativecommons.org/licenses/by/4.0/).
- Attribution: “Translators' Brief lexicon of Extended Strong's (TBESH/TBESG),
  STEPBible.org, CC BY 4.0.”

Definitions are cleaned from source HTML into plain text and shortened where
needed for the TUI. The short codes and lexical meaning are preserved.

## Downloadable translations

The optional `.tbp` packs are built from public-domain eBible.org USFM
editions. Their exact SHA-256 hashes are recorded in
`internal/catalog/data/catalog.json`.

| ID | Translation | Source | Notes |
| --- | --- | --- | --- |
| `asv` | American Standard Version | <https://ebible.org/eng-asv> | Public domain (1901) |
| `web` | World English Bible | <https://ebible.org/eng-web> | Public domain; “World English Bible” is a trademark of eBible.org |
| `ylt` | Young's Literal Translation | <https://ebible.org/engylt> | Public domain (1898) |
| `wmb` | World Messianic Bible | <https://ebible.org/engwmb> | Public domain |

WEB and WMB print the final three verses of Romans as 14:24–26. For alignment
with the other translations and the canonical 16-chapter structure, the pack
builder moves those verses to Romans 16:25–27 and drops a footnote-only
placeholder verse. The verse text is otherwise unchanged.

The catalog lists only translations that can be redistributed as data in this
project. Licensed translations, study notes, and anything requiring a separate
EULA are deliberately excluded.

## Third-party Go modules

The Go module dependencies and their licenses are listed in `go.mod`, `go.sum`,
and the module caches. They are compiled into the binary under their respective
licenses.
