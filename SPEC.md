# assetsdb format specification

This is the authoritative specification of the assetsdb on-disk database format: the directory
layout, the `datapackage.json` JSON schema, id and license-inheritance rules, and the reserved
phase-2 generated-source design. It documents the `format` package (`github.com/jbeshir/assetsdb/format`)
as built — every key and type below matches `format/types.go`, `format/db.go`, and
`format/ingest.go` exactly.

Field-name provenance (Frictionless-standard vs. DCAT-mapped vs. custom) is researched in
`/workspace/refs/FINDINGS-frictionless.md`; this document restates only the conclusions needed to
use the format.

## 1. On-disk layout

```
<db>/
  datapackage.json          # REQUIRED. The index — see §2.
  sources/<sourceid>.zip    # One ZIP per curated source, stored verbatim as fetched from
                             # upstream (Kenney or KayKit). <sourceid> matches Source.Name /
                             # PackSpec.ID. Item.Path values are paths WITHIN this ZIP.
  generated/                # RESERVED for phase-2 (§5). v1 writes only a .gitkeep + note here;
                             # no code path in this repository reads or writes files under it.
```

There is no other required top-level state. A `DB` (`format.Read`) is a read-only in-memory view
of `datapackage.json`; item bytes are extracted on demand from `sources/<id>.zip` via `DB.Open`.

## 2. `datapackage.json` schema

The file is a **Frictionless Data Package v2** descriptor (https://datapackage.org/standard/data-package/):
it validates against a stock Frictionless reader because every catalogued asset is a standard Data
Resource inside the REQUIRED `resources[]` array. All non-standard fields carry the `x_assetsdb:`
prefix — the spec explicitly permits arbitrary extra properties on a descriptor, so this keeps the
file spec-conformant while adding assetsdb-specific structure. Asset packs are grouped in a custom
`x_assetsdb:sources[]` registry rather than Frictionless's own `sources` key, because Frictionless
`sources` means something different (data provenance, `{title, path, email}`) — see
FINDINGS-frictionless.md §4's naming-clash note.

### 2.1 Top-level (`DataPackage`)

| JSON key | Go field | Required | Provenance | Notes |
|---|---|---|---|---|
| `name` | `Name` | yes | Frictionless-standard | Package machine name, e.g. `"assetsdb"`. |
| `title` | `Title` | yes | Frictionless-standard | Human-readable title. |
| `version` | `Version` | yes | Frictionless-standard | SemVer of the format **content** (not the schema — see §3). |
| `created` | `Created` | yes | Frictionless-standard (maps to DCAT `dcterms:issued`) | ISO 8601 build timestamp. |
| `licenses` | `Licenses` | yes | Frictionless-standard | Package-default license list; see §2.4. All v1 content is CC0-1.0. |
| `x_assetsdb:schemaVersion` | `SchemaVersion` | yes | custom | Format version; see §3. |
| `x_assetsdb:sources` | `Sources` | yes | custom (DCAT: array of `dcat:Dataset`) | The pack registry; see §2.2. |
| `resources` | `Resources` | yes, REQUIRED by spec | Frictionless-standard (DCAT: array of `dcat:Distribution`) | Every catalogued item; see §2.3. |

### 2.2 Source registry entry (`x_assetsdb:sources[]`, Go type `Source`)

| JSON key | Go field | Required | Provenance | Notes |
|---|---|---|---|---|
| `name` | `Name` | yes | Frictionless-standard key, reused for the source id | The pack's id, matching `sources/<name>.zip` and every `Item.Source` that references it. |
| `title` | `Title` | yes | Frictionless-standard (DCAT `dcterms:title`) | Human-readable pack name. |
| `description` | `Description` | no | Frictionless-standard (DCAT `dcterms:description`) | |
| `path` | `Path` | yes | Frictionless-standard (DCAT `dcat:downloadURL`, repurposed) | Location of the stored pack, `sources/<name>.zip`. |
| `licenses` | `Licenses` | no | Frictionless-standard | Source-level license; see §2.4 for inheritance. |
| `x_assetsdb:type` | `Type` | yes | custom | `"curated"` (v1) or `"generated"` (reserved, §5). |
| `x_assetsdb:origin` | `Origin` | no | custom | Upstream pack page URL (Kenney) or repo URL (KayKit). |
| `x_assetsdb:tags` | `Tags` | no | custom (loosely DCAT `dcat:keyword`) | Theme/tag list carried over from the manifest. |

### 2.3 Item entry (`resources[]`, Go type `Item`)

| JSON key | Go field | Required | Provenance | Notes |
|---|---|---|---|---|
| `name` | `Name` | yes, REQUIRED by spec | Frictionless-standard | The Frictionless resource slug — see the naming rule below. **Distinct from `x_assetsdb:id`.** |
| `path` | `Path` | yes, REQUIRED by spec | Frictionless-standard (DCAT `dcat:downloadURL`) | Path **within** the owning source's ZIP (`sources/<Item.Source>.zip`). |
| `format` | `Format` | no | Frictionless-standard (DCAT `dcterms:format`) | File extension, no dot (`png`, `fbx`, ...). |
| `mediatype` | `MediaType` | no | Frictionless-standard (DCAT `dcat:mediaType`) | Best-effort IANA media type; omitted if unknown. Note the all-lowercase spelling — not `mediaType`. |
| `bytes` | `Bytes` | no | Frictionless-standard (DCAT `dcat:byteSize`) | File size; omitted for atlas sub-sprites (they share their atlas PNG's bytes). |
| `hash` | `Hash` | no | Frictionless-standard | `sha256:<hex>` of the file's bytes. |
| `title` | `Title` | no | Frictionless-standard (DCAT `dcterms:title`) | Humanized name. |
| *(no `licenses` key)* | — | — | — | An `Item` never carries its own license field. Resolve the effective license via `DB.LicenseFor`, which walks item → source → package (§2.4). |
| `x_assetsdb:id` | `ID` | yes | custom | Canonical stable global id; see the id-form rule below. |
| `x_assetsdb:source` | `Source` | yes | custom | Owning `Source.Name`, resolvable via `DB.SourceByID`. |
| `x_assetsdb:kind` | `Kind` | yes | custom | One of `sprite2d` \| `model3d` \| `audio` \| `font` (`format.Kind`). |
| `x_assetsdb:tokens` | `Tokens` | no | custom | Lowercase, de-duplicated search tokens from `format.Tokenize`. |
| `x_assetsdb:region` | `Region` | no | custom | `{x,y,width,height}` atlas sub-region; present only for a Kenney atlas sub-sprite, absent for a whole-file item. |
| `x_assetsdb:provenance` | `Provenance` | no (always `null` in v1) | custom | RESERVED for phase-2; see §5. Every curated item leaves this `null`. |

### 2.4 License entry (`License`, used at both package and source level)

| JSON key | Go field | Notes |
|---|---|---|
| `name` | `Name` | SPDX identifier, e.g. `"CC0-1.0"`. At least one of `name`/`path` is expected. |
| `path` | `Path` | URL to the license text. |
| `title` | `Title` | Optional human-readable name. |

**Inheritance**: an `Item` has no `licenses` field of its own — Frictionless's inheritance rule
(a resource without `licenses` inherits its dataset's) is implemented by `DB.LicenseFor(item)`,
which returns the owning `Source`'s first license entry if present, else the package's first
license entry, else the zero `License`. All v1 sources set `CC0-1.0` at the source level, so in
practice every v1 item resolves to CC0-1.0 regardless of package default.

### 2.5 Canonical item id vs. Frictionless `name`

These are two different identifiers on the same `Item` and must not be conflated:

- **`x_assetsdb:id`** is the stable, globally unique lookup key:
  `assetsdb:<sourceid>/<path-in-source>` for a whole-file item, or
  `assetsdb:<sourceid>/<atlas-path>#<subtexture-name>` for a Kenney atlas sub-sprite (the `#`
  suffix names the `<SubTexture name="...">` the item was cut from). This id never changes across
  a database rebuild as long as the source pack and the item's path/sub-sprite name don't change.
- **`name`** is the Frictionless resource slug, REQUIRED to be unique and match
  `^[a-z0-9._-]+$`-style. It is derived by lowercasing, replacing any run of characters outside
  `[a-z0-9._-]` with `-`, and de-duplicating collisions with `-2`, `-3`, ... suffixes. It exists
  only so the file validates as a real Data Package; consumers should look items up by
  `x_assetsdb:id`, not `name`.

### 2.6 Minimal worked example

```jsonc
{
  "name": "assetsdb",
  "title": "assetsdb asset library",
  "version": "1.0.0",
  "created": "2026-07-18T00:00:00Z",
  "licenses": [
    { "name": "CC0-1.0",
      "path": "https://creativecommons.org/publicdomain/zero/1.0/legalcode",
      "title": "Creative Commons Zero v1.0 Universal" }
  ],
  "x_assetsdb:schemaVersion": 1,
  "x_assetsdb:sources": [
    {
      "name": "kenney-space-shooter-extension",
      "title": "Kenney Space Shooter Extension",
      "path": "sources/kenney-space-shooter-extension.zip",
      "licenses": [ { "name": "CC0-1.0",
        "path": "https://creativecommons.org/publicdomain/zero/1.0/legalcode",
        "title": "Creative Commons Zero v1.0 Universal" } ],
      "x_assetsdb:type": "curated",
      "x_assetsdb:origin": "https://kenney.nl/assets/space-shooter-extension",
      "x_assetsdb:tags": ["space", "shooter", "2d"]
    }
  ],
  "resources": [
    {
      "name": "kenney-space-shooter-extension-spacebuilding-001",
      "path": "Spritesheet/spaceShooter2_spritesheet.png",
      "format": "png",
      "mediatype": "image/png",
      "title": "Space Building 001",
      "x_assetsdb:id": "assetsdb:kenney-space-shooter-extension/Spritesheet/spaceShooter2_spritesheet.png#spaceBuilding_001.png",
      "x_assetsdb:source": "kenney-space-shooter-extension",
      "x_assetsdb:kind": "sprite2d",
      "x_assetsdb:tokens": ["space", "building", "001", "1"],
      "x_assetsdb:region": { "x": 0, "y": 0, "width": 34, "height": 44 },
      "x_assetsdb:provenance": null
    }
  ]
}
```

## 3. Schema versioning

`x_assetsdb:schemaVersion` is `1` for everything described in this document. It versions the
**on-disk shape** (field names, required-ness, layout) — independent of `version`, which is a
SemVer of the package's *content* (e.g. bumps when packs are added/removed/rebuilt). A future
breaking change to the JSON shape or layout bumps `schemaVersion`; `format.Read` may use it to
reject or migrate an incompatible on-disk database. Additive, backward-compatible changes (a new
optional `x_assetsdb:` key) do not require a bump.

## 4. Go API (`format` package)

```go
func Classify(path string) (kind Kind, ok bool)   // extension -> Kind; ok=false for ignored files
func MediaType(path string) string                // best-effort IANA media type, "" if unknown
func Tokenize(path string) []string                // lowercase, de-duplicated search tokens

func Read(dir string) (*DB, error)
func Write(dir string, dp *DataPackage) error      // atomic: temp file + rename

func (db *DB) Search(query string) []Item
func (db *DB) SourceByID(id string) (Source, bool)
func (db *DB) LicenseFor(it Item) License
func (db *DB) Open(it Item) (io.ReadCloser, error)

type Ingester interface {
    Name() string
    Ingest(ctx context.Context, spec PackSpec) (src Source, items []Item, zip []byte, err error)
}
type PackSpec struct { ID, Name, Source, URL, Theme string; Tags []string; License string }
```

`ErrNotFound` is returned (wrapped with `%w`) by `DB.Open` when the source archive or the entry
within it doesn't exist; check with `errors.Is(err, format.ErrNotFound)`.

## 5. Phase-2: generated sources (reserved, not implemented in v1)

The format and storage layout are designed to accommodate locally-generated assets (e.g.
AI-generated sprites) without a breaking schema change, but **no code path in this repository
implements ingestion, writing, or the CLI command for it yet**. This section documents the
reserved design so a future phase has a fixed contract to build against.

- A generated source has `x_assetsdb:type: "generated"` on its `Source` entry (vs. `"curated"`
  for Kenney/KayKit packs).
- Unlike a curated source — whose bytes are a single verbatim `sources/<id>.zip` — a generated
  source stores **loose files**, not a ZIP, under `generated/<collection>/...`. This reflects that
  generated assets are added incrementally one at a time (`assetsdb add`), not fetched as a
  pre-built archive.
- Every generated `Item` fills in `x_assetsdb:provenance` (the `Provenance` Go type, already
  defined in `format/types.go`, always `nil` on every v1/curated item):

  ```jsonc
  "x_assetsdb:provenance": {
    "generator": "...",         // e.g. the tool/model family that produced the asset
    "prompt": "...",            // the generation prompt, if any
    "model": "...",             // the specific model used
    "params": { "...": "..." }, // generation parameters (seed, size, etc.)
    "createdAt": "...",         // ISO 8601 generation timestamp
    "licenseAssertion": "..."   // the asserted license/ownership basis for this asset
  }
  ```

- The reserved CLI entry point is `assetsdb add` — not implemented; see `cmd/assetsdb` for its
  current stub status.
- Nothing above requires a `schemaVersion` bump to implement: `x_assetsdb:type: "generated"`,
  loose files under `generated/`, and non-null `x_assetsdb:provenance` are all already valid
  under the schema described in §2.

## 6. How `mcp-servers/assets` consumes this

The `assets` MCP server is expected to import the `format` package directly and read the
database location from an environment variable:

```go
import (
    "os"

    "github.com/jbeshir/assetsdb/format"
)

db, err := format.Read(os.Getenv("ASSETS_DB"))
if err != nil {
    // handle error
}

items := db.Search("dungeon torch")
for _, it := range items {
    lic := db.LicenseFor(it)
    rc, err := db.Open(it)
    // stream rc to the caller, close when done
}
```

No HTTP, RPC, or serialization layer sits between `assetsdb` and its consumers in v1 — the
database is a plain directory on disk, read in-process via the `format` package.
