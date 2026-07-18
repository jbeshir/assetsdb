package format

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned when a requested item, source archive, or archive entry cannot be
// located. Wrap it with %w and check with errors.Is.
var ErrNotFound = errors.New("assetsdb: item not found")

const datapackageFile = "datapackage.json"

// DB is an in-memory, read-only view of a datapackage.json loaded from disk. It indexes sources
// by name for fast lookup and can extract an item's bytes from its source archive via Open.
type DB struct {
	dir     string
	pkg     *DataPackage
	sources map[string]Source
}

// Read loads <dir>/datapackage.json into memory and returns a DB ready for lookups.
func Read(dir string) (*DB, error) {
	// #nosec G304 -- dir is the caller-supplied database directory path
	data, err := os.ReadFile(filepath.Join(dir, datapackageFile))
	if err != nil {
		return nil, fmt.Errorf("assetsdb: read %s: %w", datapackageFile, err)
	}

	var dp DataPackage
	if err := json.Unmarshal(data, &dp); err != nil {
		return nil, fmt.Errorf("assetsdb: parse %s: %w", datapackageFile, err)
	}

	sources := make(map[string]Source, len(dp.Sources))
	for _, s := range dp.Sources {
		sources[s.Name] = s
	}

	return &DB{dir: dir, pkg: &dp, sources: sources}, nil
}

// Write atomically writes dp to <dir>/datapackage.json: it marshals to a temporary file in dir
// and renames it into place, so a concurrent reader never observes a partially-written file.
func Write(dir string, dp *DataPackage) error {
	data, err := json.MarshalIndent(dp, "", "  ")
	if err != nil {
		return fmt.Errorf("assetsdb: marshal %s: %w", datapackageFile, err)
	}

	tmp, err := os.CreateTemp(dir, ".datapackage-*.json.tmp")
	if err != nil {
		return fmt.Errorf("assetsdb: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if werr := writeAndClose(tmp, data); werr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("assetsdb: write temp file: %w", werr)
	}

	if err := os.Rename(tmpName, filepath.Join(dir, datapackageFile)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("assetsdb: rename temp file into place: %w", err)
	}

	return nil
}

func writeAndClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Search returns every item whose title and search tokens contain, as substrings, every
// whitespace-separated token of query. Matching is case-insensitive; an empty query matches
// every item.
func (db *DB) Search(query string) []Item {
	queryTokens := strings.Fields(strings.ToLower(query))

	var hits []Item
	for _, it := range db.pkg.Resources {
		haystack := strings.ToLower(it.Title + " " + strings.Join(it.Tokens, " "))
		if matchesAllTokens(haystack, queryTokens) {
			hits = append(hits, it)
		}
	}

	return hits
}

func matchesAllTokens(haystack string, tokens []string) bool {
	for _, t := range tokens {
		if !strings.Contains(haystack, t) {
			return false
		}
	}
	return true
}

// SourceByID returns the Source registered under id, and whether it was found.
func (db *DB) SourceByID(id string) (Source, bool) {
	s, ok := db.sources[id]
	return s, ok
}

// LicenseFor resolves the effective license for it via the Frictionless inheritance chain:
// items never carry their own license (see the Item doc), so this walks item -> source ->
// package licenses and returns the first non-empty license list's first entry. It returns the
// zero License if neither the source nor the package declares one.
func (db *DB) LicenseFor(it Item) License {
	if src, ok := db.SourceByID(it.Source); ok && len(src.Licenses) > 0 {
		return src.Licenses[0]
	}
	if len(db.pkg.Licenses) > 0 {
		return db.pkg.Licenses[0]
	}
	return License{}
}

// Open extracts it from its source archive (<dir>/sources/<it.Source>.zip) at it.Path and
// returns a ReadCloser over its bytes. The returned error wraps ErrNotFound if the source
// archive or the entry within it does not exist.
func (db *DB) Open(it Item) (io.ReadCloser, error) {
	zipPath := filepath.Join(db.dir, "sources", it.Source+".zip")

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("assetsdb: open source archive %q: %w", it.Source, ErrNotFound)
		}
		return nil, fmt.Errorf("assetsdb: open source archive %q: %w", it.Source, err)
	}

	for _, f := range zr.File {
		if f.Name != it.Path {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			_ = zr.Close()
			return nil, fmt.Errorf("assetsdb: open entry %q: %w", it.Path, err)
		}

		return &zipEntryReadCloser{rc: rc, zr: zr}, nil
	}

	_ = zr.Close()
	return nil, fmt.Errorf("assetsdb: entry %q not found in source %q: %w", it.Path, it.Source, ErrNotFound)
}

// zipEntryReadCloser closes both a zip entry's reader and its parent zip.ReadCloser together, so
// callers only need to Close the value Open returns.
type zipEntryReadCloser struct {
	rc io.ReadCloser
	zr *zip.ReadCloser
}

// Read implements io.Reader by delegating to the underlying zip entry reader.
func (z *zipEntryReadCloser) Read(p []byte) (int, error) { return z.rc.Read(p) }

// Close closes the zip entry reader and its parent archive, returning the first error from
// either.
func (z *zipEntryReadCloser) Close() error {
	rcErr := z.rc.Close()
	zrErr := z.zr.Close()
	if rcErr != nil {
		return rcErr
	}
	return zrErr
}
