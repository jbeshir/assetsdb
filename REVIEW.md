# assetsdb: module-root consumer API — code review

**Scope:** uncommitted changes in `/workspace/assetsdb` vs baseline `e3e5acb`, excluding
`packs.yaml` (protected, caller-owned). Includes untracked files. Diff reviewed:
`README.md`, `SPEC.md`, `format/db.go` (deleted), `format/db_test.go` (deleted),
`internal/build/build.go`, `internal/build/build_test.go`, plus untracked
`assetsdb.go`, `assetsdb_test.go`, `cmd/assetsdb/main_test.go`,
`format/serialization.go`, `format/serialization_test.go`.

Independently re-verified in a clean sandbox (not just trusting the reported run):
`go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
`golangci-lint run --config .golangci.yml ./...` — all clean, 0 lint issues.

## Verdict: **PASS**

The move of the consumer DB API to the module-root `assetsdb` package is complete, the
package boundary is clean and matches the "format = schema/serialization/catalog only"
mandate, path-containment/symlink handling is correct and tested adversarially, copy-safety
and ordering guarantees are implemented and tested, `ErrNotFound` is a single typed sentinel
used consistently, and there is no duplicate operational implementation (the old
`format.Write` atomic-writer was relocated wholesale into `internal/build`, not copied).
Docs and CLI are updated appropriately. Only minor, non-blocking findings below.

---

## Findings (prioritized; all low severity, none blocking)

### 1. Stale hardcoded-path description in SPEC.md §1 (readability/doc accuracy)
`SPEC.md:28-29` still says:

> A root `assetsdb.DB` (`assetsdb.Read`) is a read-only in-memory view of `datapackage.json`;
> item bytes are extracted on demand from `sources/<id>.zip` via `DB.Open`.

This describes the *old*, hardcoded `<dir>/sources/<source>.zip` convention. The actual (and
correctly implemented) behavior — resolving each item's bytes via the registered
`Source.Path`, with root-containment/symlink checks — is documented accurately later in the
same file (`SPEC.md:187-191`). The two passages now disagree with each other. Suggest
rewording line 29 to something like "item bytes are extracted on demand from the archive
registered on the item's `Source` (see §4) via `DB.Open`" so a reader skimming §1 doesn't
walk away with the superseded mental model.

### 2. Atomicity test coverage was dropped in the Write relocation (tests)
Baseline `format/db_test.go` had `TestWriteIsAtomic`, which asserted the output directory
contains *exactly one* file after a write (i.e., no leftover `.datapackage-*.json.tmp`).
`format.Write` was correctly relocated (not duplicated) into
`internal/build/build.go:118-137` as `writeDataPackage`, but no equivalent assertion moved
with it — `internal/build/build_test.go`'s `TestBuild` (line 119) only checks that
`datapackage.json` exists, not that no stray temp file remains alongside it. The
implementation itself looks correct (temp file + rename, cleaned up on error), so this is a
coverage gap rather than a bug; consider adding a directory-listing assertion to `TestBuild`
(or a small dedicated test) to keep that guarantee explicit and regression-tested.

### 3. `assetsdb.DataPackage` alias is unused API surface (naming/completeness, nit)
`assetsdb.go:19` exports `DataPackage = format.DataPackage` alongside the aliases that *are*
needed for the public method signatures (`Item`, `Kind`, `License`, `Provenance`, `Region`,
`Source`). Unlike those, no `DB` method returns or accepts a `DataPackage` — it's not part of
the consumer surface described in the task (`DB, Read, Search, LicenseFor, SourceByID, Open,
Sources, ItemsForSource, OpenSource, ErrNotFound`). A caller who genuinely needs the raw
`DataPackage` type already has to import `format` (to call `Decode`/`Encode`), so this alias
doesn't save them anything and slightly widens the module-root surface beyond what the task
scoped. Consider dropping it, or if it's intentionally kept for forward compatibility, a short
comment saying why would help a future maintainer understand it's deliberate rather than
leftover.

### 4. `Search`'s ordering/copy-safety story is weaker than `Sources`/`ItemsForSource` (readability, nit)
`Sources()` and `ItemsForSource()` both sort by stable ID and always return a copy-safe,
non-nil slice (`assetsdb.go:142-166`), and both are directly tested for it. `Search`
(`assetsdb.go:116-128`) also returns caller-owned clones (good — no aliasing bug), but it
iterates `db.dataPackage.Resources` in on-disk order rather than the sorted index, and returns
`nil` rather than an empty slice when there are no hits. In practice this is deterministic
because `internal/build.Build` always writes `Resources` pre-sorted by item ID
(`internal/build/build.go:97`), so today's on-disk databases are stable — but that determinism
is an incidental property of the writer, not something `Search` enforces itself the way the
other two accessors do. Not a bug, but worth either a one-line doc comment on `Search`
clarifying that its result order tracks on-disk resource order (not a DB-enforced sort), or
sorting for defense-in-depth against hand-edited `datapackage.json` files.

---

## Assessment by review dimension

**API naming/completeness & package boundary** — Exactly the surface the task asked for is
present and correctly named: `DB`, `Read`, `Search`, `LicenseFor`, `SourceByID`, `Open`,
`Sources`, `ItemsForSource`, `OpenSource`, `ErrNotFound`. The `format` package now truly holds
only schema (`types.go`), serialization (new `serialization.go`: `Decode`/`Encode`), catalog
construction (`catalog.go`), classification (`classify.go`), tokenization (`tokenize.go`), and
the `Ingester` contract (`ingest.go`) — no `Read`/`DB`/`Open` remain there (verified via grep;
zero stray references to `format.Read`/`format.DB`/`format.ErrNotFound`/etc. anywhere in the
tree). `OpenSource` is clearly distinguished in name and doc from `Open` (item extraction vs.
raw archive), avoiding ambiguity between "open an item" and "open a source archive."

**Backwards compatibility vs. duplication** — The old `format.Write` atomic-writer was moved,
not copied: `grep` confirms no `Write` function remains in `format`, and `internal/build`'s
`writeDataPackage`/`encodeAndClose` is the sole atomic-write implementation in the tree. There
is exactly one implementation of the read path (`assetsdb.Read`) and one of the write path
(`internal/build.Build`), consistent with "no duplicate operational implementation." Breaking
the old `format.Read`/`format.DB` consumer surface was an explicit part of the task (move to
module root), and both call sites that depended on it (`internal/build/build_test.go`,
`cmd/assetsdb` indirectly via docs) were updated; nothing was left half-migrated.

**Path traversal & symlink security** — `registeredPath` (`assetsdb.go:226-249`) is correct:
rejects empty and absolute registered paths, rejects lexical escapes via `filepath.Join` +
`isWithin` *before* touching the filesystem, resolves symlinks with `filepath.EvalSymlinks`,
and re-checks containment against the resolved real path — closing the classic
join-then-check-then-symlink-swap gap. This is exercised by real adversarial tests:
`TestRegisteredSourcePathContainment` (parent traversal and absolute-path cases) and
`TestRegisteredSourceRejectsEscapingSymlink` (registered path is a symlink pointing outside
root). `db.root` itself is symlink-resolved once at `Read` time (`assetsdb.go:56-59`), so all
later containment comparisons are between two canonical paths — no TOCTOU-shaped gap from a
mutable root.

**File descriptor / error behavior** — `openSource` distinguishes "unknown source name" (wraps
`ErrNotFound` immediately) from "registered path missing on disk" (`os.ErrNotExist` from
`os.Open`, mapped to `ErrNotFound`) from other I/O errors (passed through unwrapped-but-
wrapped-with-context). `itemReadCloser.Close` closes both the zip entry and the underlying
`*os.File` and doesn't leak the archive fd if the entry-close fails. `Open` closes the archive
on the entry-open error path (`assetsdb.go:189-192`) so a bad `item.Path` doesn't leak an fd.
`bodyclose`/`errcheck`-class concerns are covered by the passing lint run.

**Copy safety & deterministic ordering** — `Sources()` sorts by `Name` at index-build time and
deep-clones (`Licenses`, `Tags`) on every call; `ItemsForSource()` sorts by `ID` at index-build
time and deep-clones (`Tokens`, `Region`, `Provenance.Params`) on every call; `SourceByID`
likewise clones. All three are directly tested for both stable order and mutation-safety
(`TestSourceEnumerationIsOrderedAndMutationSafe`, `TestItemEnumerationIsOrderedAndMutationSafe`)
by mutating a returned value and re-fetching to confirm the internal index didn't move. See
Finding 4 for the one asymmetry (`Search`'s ordering isn't self-enforced the same way).

**Tests, docs, CLI** — Test coverage is a net improvement over baseline: the new
`assetsdb_test.go` covers everything `format/db_test.go` did plus containment/symlink attacks,
`OpenSource`, and copy/order guarantees the old tests didn't check. `format/serialization_test.go`
covers the new `Decode`/`Encode` round-trip and decode-error path. `internal/build/build_test.go`
was updated to import `assetsdb` instead of `format` for its `DB` usage, and
`TestLoadManifestRealPacksYAML` was loosened from an exact pack count to a "contains these IDs"
check plus `GreaterOrEqual`, appropriately tolerant of the caller-owned `packs.yaml` growing over
time without the test needing to track its size. `cmd/assetsdb/main_test.go` adds coverage for
`printUsage` that didn't exist before (CLI itself doesn't use the DB API directly — it only
builds — so no CLI code changes were required by the package move, and none were made). `README.md`
and `SPEC.md` were both updated to reflect the new import path, full method surface, and security
properties (bar Finding 1's stale line).

**Readability / architecture** — `assetsdb.go` is organized in a clear top-to-bottom order:
type aliases → `DB` struct → `Read`/indexing → query methods (`Search`, `Sources`,
`SourceByID`, `ItemsForSource`, `LicenseFor`) → extraction (`Open`, `OpenSource`,
`openSource`, `registeredPath`, `isWithin`, `openZipEntry`) → `itemReadCloser` →
clone helpers, with blank-line separation between each concern. Functions are short and each
does one thing (`registeredPath` itself is the most complex function in the diff at ~20 lines
across 4 checks, comfortably under the repo's configured `gocyclo` threshold of 12, confirmed
by the clean lint run). Naming is unambiguous: `Open` vs `OpenSource`, `registeredPath` vs
`isWithin`, `cloneSource`/`cloneSources`/`cloneItem`/`cloneMap` read as a clear escalating
family. Ownership is clear — `format` owns shape/serialization/catalog-building, `assetsdb`
owns runtime read access, `internal/build` owns the write/orchestration path — and nothing
in the diff blurs those lines.

## Post-review remediation

The implementer addressed all four non-blocking findings in one cycle: corrected the stale
archive-path description, restored atomic-writer cleanup coverage, removed the unused root
`DataPackage` alias, and made `Search` return non-nil copy-safe results sorted by stable item ID.
