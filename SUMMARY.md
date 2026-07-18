# Implementation summary

The public, consumer-facing database API now lives at the importable module root as package
`assetsdb`. It provides `DB`, `Read`, `Search`, `LicenseFor`, `SourceByID`, `Open`, `Sources`,
`ItemsForSource`, `OpenSource`, and the typed `ErrNotFound` sentinel. Public item, source,
license, kind, region, and provenance names are aliases of the on-disk schema types.

The `format` package now owns schema types, JSON encoding/decoding, catalog construction,
classification, tokenization, and ingest contracts only. Its former filesystem-root database
implementation was removed rather than retained as a duplicate. Atomic database index writing
is owned by the internal build package.

Source and item enumeration, plus search results, are sorted by stable IDs and defensively
copied. Raw archives and item entries are opened through each registered `Source.Path`. Both
lexical traversal and symlink escapes outside the canonical database root are rejected, while
unknown sources, missing archives, and missing entries consistently wrap `assetsdb.ErrNotFound`.

README, SPEC, internal consumers, build tests, and CLI coverage were updated. The CLI remains at
`cmd/assetsdb` and builds through the existing Makefile target.

The caller-owned `packs.yaml` change was preserved byte-for-byte and excluded from the commit.
