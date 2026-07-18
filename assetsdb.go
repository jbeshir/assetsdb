// Package assetsdb provides read-only access to an assetsdb database directory.
package assetsdb

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jbeshir/assetsdb/format"
)

// Format types are aliases so consumers can use the database API without importing format.
type (
	Item       = format.Item
	Kind       = format.Kind
	License    = format.License
	Provenance = format.Provenance
	Region     = format.Region
	Source     = format.Source
)

const (
	KindSprite2D = format.KindSprite2D
	KindModel3D  = format.KindModel3D
	KindAudio    = format.KindAudio
	KindFont     = format.KindFont
)

// ErrNotFound identifies an unknown source, a missing registered archive, or a missing item.
var ErrNotFound = errors.New("assetsdb: not found")

const dataPackageFile = "datapackage.json"

// DB is a read-only, in-memory index of an assetsdb database directory.
type DB struct {
	root          string
	dataPackage   *format.DataPackage
	sources       []format.Source
	items         []format.Item
	sourcesByID   map[string]format.Source
	itemsBySource map[string][]format.Item
}

// Read loads and indexes the datapackage.json beneath root.
func Read(root string) (*DB, error) {
	canonicalRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("assetsdb: resolve database root: %w", err)
	}

	canonicalRoot, err = filepath.EvalSymlinks(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("assetsdb: resolve database root: %w", err)
	}

	dataPackage, err := readDataPackage(canonicalRoot)
	if err != nil {
		return nil, err
	}

	return indexDataPackage(canonicalRoot, dataPackage), nil
}

func readDataPackage(root string) (*format.DataPackage, error) {
	path := filepath.Join(root, dataPackageFile)

	// #nosec G304 -- path is constrained to the caller-selected database root.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("assetsdb: open %s: %w", dataPackageFile, err)
	}
	defer func() { _ = file.Close() }()

	dataPackage, err := format.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("assetsdb: read %s: %w", dataPackageFile, err)
	}

	return dataPackage, nil
}

func indexDataPackage(root string, dataPackage *format.DataPackage) *DB {
	sources := cloneSources(dataPackage.Sources)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	items := cloneItems(dataPackage.Resources)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	db := &DB{
		root:          root,
		dataPackage:   dataPackage,
		sources:       sources,
		items:         items,
		sourcesByID:   make(map[string]format.Source, len(sources)),
		itemsBySource: make(map[string][]format.Item),
	}

	for _, source := range sources {
		db.sourcesByID[source.Name] = source
	}

	for _, item := range items {
		db.itemsBySource[item.Source] = append(db.itemsBySource[item.Source], item)
	}

	return db
}

// Search returns matching items ordered by stable item ID.
func (db *DB) Search(query string) []Item {
	queryTokens := strings.Fields(strings.ToLower(query))

	matches := make([]format.Item, 0)
	for _, item := range db.items {
		haystack := strings.ToLower(item.Title + " " + strings.Join(item.Tokens, " "))
		if containsAll(haystack, queryTokens) {
			matches = append(matches, cloneItem(item))
		}
	}

	return matches
}

func containsAll(haystack string, tokens []string) bool {
	for _, token := range tokens {
		if !strings.Contains(haystack, token) {
			return false
		}
	}

	return true
}

// Sources returns all registered sources ordered by source ID.
// The returned values do not share mutable slices with the database.
func (db *DB) Sources() []Source {
	return cloneSources(db.sources)
}

// SourceByID returns a mutation-safe copy of the registered source with id.
func (db *DB) SourceByID(id string) (Source, bool) {
	source, ok := db.sourcesByID[id]
	if !ok {
		return Source{}, false
	}

	return cloneSource(source), true
}

// ItemsForSource returns the source's items ordered by stable item ID.
// Unknown sources and sources with no items both return an empty slice.
func (db *DB) ItemsForSource(sourceID string) []Item {
	items := db.itemsBySource[sourceID]
	result := make([]format.Item, len(items))
	for i, item := range items {
		result[i] = cloneItem(item)
	}

	return result
}

// LicenseFor resolves an item's first source license, then the package's first license.
func (db *DB) LicenseFor(item Item) License {
	if source, ok := db.sourcesByID[item.Source]; ok && len(source.Licenses) > 0 {
		return source.Licenses[0]
	}

	if len(db.dataPackage.Licenses) > 0 {
		return db.dataPackage.Licenses[0]
	}

	return License{}
}

// Open extracts item.Path from the archive registered by the item's source.
func (db *DB) Open(item Item) (io.ReadCloser, error) {
	archive, err := db.openSource(item.Source)
	if err != nil {
		return nil, err
	}

	entry, err := openZipEntry(archive, item.Path)
	if err != nil {
		_ = archive.Close()
		return nil, fmt.Errorf("assetsdb: open item %q: %w", item.ID, err)
	}

	return &itemReadCloser{entry: entry, archive: archive}, nil
}

// OpenSource returns the raw bytes of the archive registered under sourceID.
// The caller must close the returned reader.
func (db *DB) OpenSource(sourceID string) (io.ReadCloser, error) {
	return db.openSource(sourceID)
}

func (db *DB) openSource(sourceID string) (*os.File, error) {
	source, ok := db.sourcesByID[sourceID]
	if !ok {
		return nil, fmt.Errorf("assetsdb: source %q: %w", sourceID, ErrNotFound)
	}

	archivePath, err := db.registeredPath(source.Path)
	if err != nil {
		return nil, fmt.Errorf("assetsdb: source %q: %w", sourceID, err)
	}

	// #nosec G304 -- registeredPath verifies lexical and symlink containment.
	archive, err := os.Open(archivePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("assetsdb: source %q archive: %w", sourceID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("assetsdb: open source %q archive: %w", sourceID, err)
	}

	return archive, nil
}

func (db *DB) registeredPath(registeredPath string) (string, error) {
	if registeredPath == "" || filepath.IsAbs(filepath.FromSlash(registeredPath)) {
		return "", fmt.Errorf("registered path %q escapes database root", registeredPath)
	}

	candidate := filepath.Join(db.root, filepath.FromSlash(registeredPath))
	if !isWithin(db.root, candidate) {
		return "", fmt.Errorf("registered path %q escapes database root", registeredPath)
	}

	resolved, err := filepath.EvalSymlinks(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve registered path %q: %w", registeredPath, err)
	}

	if !isWithin(db.root, resolved) {
		return "", fmt.Errorf("registered path %q escapes database root", registeredPath)
	}

	return resolved, nil
}

func isWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}

	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func openZipEntry(archive *os.File, entryPath string) (io.ReadCloser, error) {
	info, err := archive.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat archive: %w", err)
	}

	reader, err := zip.NewReader(archive, info.Size())
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}

	for _, entry := range reader.File {
		if entry.Name == entryPath {
			return entry.Open()
		}
	}

	return nil, fmt.Errorf("entry %q: %w", entryPath, ErrNotFound)
}

type itemReadCloser struct {
	entry   io.ReadCloser
	archive *os.File
}

func (reader *itemReadCloser) Read(buffer []byte) (int, error) {
	return reader.entry.Read(buffer)
}

func (reader *itemReadCloser) Close() error {
	entryErr := reader.entry.Close()
	archiveErr := reader.archive.Close()
	if entryErr != nil {
		return entryErr
	}

	return archiveErr
}

func cloneSources(sources []format.Source) []format.Source {
	result := make([]format.Source, len(sources))
	for i, source := range sources {
		result[i] = cloneSource(source)
	}

	return result
}

func cloneItems(items []format.Item) []format.Item {
	result := make([]format.Item, len(items))
	for i, item := range items {
		result[i] = cloneItem(item)
	}

	return result
}

func cloneSource(source format.Source) format.Source {
	source.Licenses = append([]format.License(nil), source.Licenses...)
	source.Tags = append([]string(nil), source.Tags...)

	return source
}

func cloneItem(item format.Item) format.Item {
	item.Tokens = append([]string(nil), item.Tokens...)

	if item.Region != nil {
		region := *item.Region
		item.Region = &region
	}

	if item.Provenance != nil {
		provenance := *item.Provenance
		provenance.Params = cloneMap(item.Provenance.Params)
		item.Provenance = &provenance
	}

	return item
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}

	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}

	return result
}
