# assetsdb

`assetsdb` builds and provides searchable access to an on-disk database of CC0-licensed game-art
assets. It ingests curated asset packs (currently [Kenney](https://kenney.nl) and
[KayKit](https://kaykit.itch.io)) into a single portable directory containing a JSON catalog and
the original source archives.

The full on-disk and JSON format is specified in [SPEC.md](SPEC.md). This README covers what
exists and how to use it.

## License: code vs. assets

This distinction matters and is easy to blur, so it's stated up front:

- The **assetsdb source code** (this repository) is licensed under the [MIT License](LICENSE).
- The **assets it ingests** (Kenney and KayKit packs) are licensed **CC0-1.0** (public domain) by
  their original authors. `assetsdb` does not relicense them; it records their CC0-1.0 license in
  the database and never mixes it up with the MIT license of assetsdb's own code.

## Install / build

```sh
make build
```

This builds the `assetsdb` CLI to `bin/assetsdb` (`./cmd/assetsdb`). `make setup-tools` installs
the pinned `golangci-lint`; `make validate` runs lint, test, and build together.

## Usage

### `assetsdb build` (implemented)

```sh
assetsdb build --manifest packs.yaml --out <db>
# Rebuild metadata from ZIPs already in <db>/sources (no downloads):
assetsdb build --manifest packs.yaml --out <db> --reuse-archives
```

Reads a manifest (see [`packs.yaml`](packs.yaml) for the starter curated list of Kenney and
KayKit packs), fetches each listed pack over the network with the matching ingester, and writes a
fresh database to `<db>` (a `datapackage.json` index plus one ZIP per source under `<db>/sources`).

### `assetsdb add` (reserved, phase-2 — not yet implemented)

`assetsdb add` is reserved for a future phase that ingests locally-generated assets (e.g.
AI-generated sprites) into the database's `generated/` directory as loose files, with a
provenance record per item. The on-disk format and Go types already accommodate this (see
[SPEC.md](SPEC.md#phase-2-generated-sources-reserved)), but no code path in this repository writes
to `generated/` or implements the `add` command yet.

## On-disk layout

```
<db>/
  datapackage.json          # the index (Frictionless Data Package v2 shaped)
  sources/<sourceid>.zip    # curated packs, stored verbatim as fetched
  generated/                # reserved for phase-2 locally-generated assets (empty in v1)
```

## Format: adopted specs

`datapackage.json` is a hybrid of three established specs rather than a bespoke schema:

- **[Frictionless Data Package v2](https://datapackage.org/standard/data-package/)** — the
  on-disk shape itself; every asset is a standard Data Resource in the required `resources[]`.
- **[W3C DCAT v3](https://www.w3.org/TR/vocab-dcat-3/)** — the semantic cross-reference
  (Catalog/Dataset/Distribution), so the file can be losslessly re-expressed as RDF later.
- **[SPDX](https://spdx.org/licenses/)** — license identifiers (`CC0-1.0`, `MIT`, ...).

See [SPEC.md](SPEC.md) for the full key-by-key schema and the Frictionless/DCAT crosswalk.

## v1 sources

Two curated ingest sources, both confirmed CC0-1.0:

- **Kenney** — page-scrape of the pack page's direct ZIP download link, plus optional parsing of
  a Sparrow/Starling-style spritesheet XML atlas (`<TextureAtlas><SubTexture>`) when a pack ships
  one.
- **KayKit** — GitHub codeload download (`codeload.github.com/.../zip/refs/heads/<branch>`) of
  each pack repo, walking the archive for 3D models and textures.

## Go package

The module root, `github.com/jbeshir/assetsdb`, contains the database API, schema types, catalog
construction helpers, JSON serialization, asset classification and tokenization, and the ingest
contract. For example:

```go
import "github.com/jbeshir/assetsdb"

db, err := assetsdb.Read(os.Getenv("ASSETS_DB"))
if err != nil {
    // handle error
}

hits := db.Search("dungeon torch")
for _, item := range hits {
    lic := db.LicenseFor(item)
    rc, err := db.Open(item)
    // ...
}

sources := db.Sources() // stable source-ID order; safe for the caller to mutate
items := db.ItemsForSource(sources[0].Name) // stable item-ID order; also mutation-safe

archive, err := db.OpenSource(sources[0].Name) // raw registered source archive
// use and close archive
```

The path passed to `Read` should be a directory built by `assetsdb build` (one containing
`datapackage.json`). `Open` and `OpenSource` use the archive path registered in each source and
reject paths that escape the database root. The root package also exposes `DataPackage`,
`Encode`, `Decode`, `Classify`, `MediaType`, `Tokenize`, `PackSpec`, and `Ingester`; see
[SPEC.md](SPEC.md) for the complete API.

Every active manifest entry declares an `extraction` policy. Ingestion keeps only assets whose
upstream filename or atlas record identifies their contents, and records that naming evidence on
the resulting resource. Packs with no deterministically identifiable items live in
[`removed-packs.yaml`](removed-packs.yaml), with their source metadata and removal reason retained.
