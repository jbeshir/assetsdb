package assetsdb_test

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jbeshir/assetsdb"
)

func TestReadSearchAndLicense(t *testing.T) {
	db, dataPackage := createTestDB(t)

	hits := db.Search("GOLD coin")
	require.Len(t, hits, 1)
	require.Equal(t, "assetsdb:source-a/a.png", hits[0].ID)
	require.Equal(t, assetsdb.License{Name: "CC0-1.0"}, db.LicenseFor(hits[0]))

	orphan := hits[0]
	orphan.Source = "unknown"
	require.Equal(t, dataPackage.Licenses[0], db.LicenseFor(orphan))
	require.NotNil(t, db.Search("not present"))
}

func TestSearchIsOrderedAndMutationSafe(t *testing.T) {
	db, _ := createTestDB(t)

	hits := db.Search("coin")
	require.Equal(t, []string{"assetsdb:source-a/a.png", "assetsdb:source-a/z.png"}, itemIDs(hits))
	hits[0].Tokens[0] = "mutated"

	again := db.Search("coin")
	require.Equal(t, "coin", again[0].Tokens[0])
}

func TestOpenItemAndRegisteredSource(t *testing.T) {
	db, dataPackage := createTestDB(t)

	itemReader, err := db.Open(itemByID(t, dataPackage, "assetsdb:source-a/a.png"))
	require.NoError(t, err)
	require.Equal(t, []byte("gold coin"), readAndClose(t, itemReader))

	sourceReader, err := db.OpenSource("source-b")
	require.NoError(t, err)
	rawArchive := readAndClose(t, sourceReader)

	archive, err := zip.NewReader(bytes.NewReader(rawArchive), int64(len(rawArchive)))
	require.NoError(t, err)
	require.Len(t, archive.File, 1)
	require.Equal(t, "audio/hit.ogg", archive.File[0].Name)
}

func TestOpenUsesRegisteredSourcePath(t *testing.T) {
	db, dataPackage := createTestDB(t)

	item := dataPackage.Resources[1]
	reader, err := db.Open(item)
	require.NoError(t, err)
	require.Equal(t, []byte("hit sound"), readAndClose(t, reader))
}

func TestOpenNotFoundErrors(t *testing.T) {
	db, dataPackage := createTestDB(t)

	_, err := db.OpenSource("unknown")
	require.ErrorIs(t, err, assetsdb.ErrNotFound)

	missingSource := dataPackage.Sources[0]
	missingSource.Name = "missing"
	missingSource.Path = "sources/missing.zip"
	missingPackage := *dataPackage
	missingPackage.Sources = append([]assetsdb.Source{missingSource}, dataPackage.Sources...)
	missingDB := readTestDB(t, t.TempDir(), &missingPackage, nil)

	_, err = missingDB.OpenSource("missing")
	require.ErrorIs(t, err, assetsdb.ErrNotFound)

	unknownSourceItem := dataPackage.Resources[0]
	unknownSourceItem.Source = "unknown"
	_, err = db.Open(unknownSourceItem)
	require.ErrorIs(t, err, assetsdb.ErrNotFound)

	missingItem := dataPackage.Resources[0]
	missingItem.Path = "missing.png"
	_, err = db.Open(missingItem)
	require.ErrorIs(t, err, assetsdb.ErrNotFound)
}

func TestRegisteredSourcePathContainment(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.zip")
	require.NoError(t, os.WriteFile(outside, buildZip(t, map[string]string{"secret": "data"}), 0o600))

	tests := []struct {
		name string
		path string
	}{
		{name: "parent traversal", path: "../outside.zip"},
		{name: "absolute", path: outside},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataPackage := testDataPackage()
			dataPackage.Sources[0].Path = test.path
			db := readTestDB(t, t.TempDir(), dataPackage, nil)

			_, err := db.OpenSource(dataPackage.Sources[0].Name)
			require.Error(t, err)
			require.NotErrorIs(t, err, assetsdb.ErrNotFound)
			require.Contains(t, err.Error(), "escapes database root")
		})
	}
}

func TestRegisteredSourceRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.zip")
	require.NoError(t, os.WriteFile(outside, buildZip(t, map[string]string{"secret": "data"}), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked.zip")))

	dataPackage := testDataPackage()
	dataPackage.Sources[0].Path = "linked.zip"
	db := readTestDB(t, root, dataPackage, nil)

	_, err := db.OpenSource(dataPackage.Sources[0].Name)
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes database root")
}

func TestSourceEnumerationIsOrderedAndMutationSafe(t *testing.T) {
	db, _ := createTestDB(t)
	const mutation = "mutated"

	sources := db.Sources()
	require.Equal(t, []string{"source-a", "source-b"}, []string{sources[0].Name, sources[1].Name})
	sources[0].Tags[0] = mutation
	sources[0].Licenses[0].Name = mutation

	again := db.Sources()
	require.Equal(t, "icons", again[0].Tags[0])
	require.Equal(t, "CC0-1.0", again[0].Licenses[0].Name)

	byID, ok := db.SourceByID("source-a")
	require.True(t, ok)
	byID.Tags[0] = mutation
	unchanged, ok := db.SourceByID("source-a")
	require.True(t, ok)
	require.Equal(t, "icons", unchanged.Tags[0])

	_, ok = db.SourceByID("unknown")
	require.False(t, ok)
}

func TestItemEnumerationIsOrderedAndMutationSafe(t *testing.T) {
	db, _ := createTestDB(t)
	const mutation = "mutated"

	items := db.ItemsForSource("source-a")
	require.Len(t, items, 2)
	require.Equal(t, "assetsdb:source-a/a.png", items[0].ID)
	require.Equal(t, "assetsdb:source-a/z.png", items[1].ID)

	items[0].Tokens[0] = mutation
	items[0].Region.X = 99
	items[0].Provenance.Params["seed"] = mutation

	again := db.ItemsForSource("source-a")
	require.Equal(t, "coin", again[0].Tokens[0])
	require.Equal(t, 1, again[0].Region.X)
	require.Equal(t, "1", again[0].Provenance.Params["seed"])
	require.Empty(t, db.ItemsForSource("unknown"))
}

func createTestDB(t *testing.T) (*assetsdb.DB, *assetsdb.DataPackage) {
	t.Helper()

	root := t.TempDir()
	archives := map[string][]byte{
		"archives/custom-a.zip": buildZip(t, map[string]string{
			"images/a.png": "gold coin",
			"images/z.png": "silver coin",
		}),
		"stored/source-b.raw": buildZip(t, map[string]string{"audio/hit.ogg": "hit sound"}),
	}

	dataPackage := testDataPackage()
	return readTestDB(t, root, dataPackage, archives), dataPackage
}

func testDataPackage() *assetsdb.DataPackage {
	return &assetsdb.DataPackage{
		Name:          "assetsdb-test",
		Title:         "Test Package",
		Version:       "1.0.0",
		Created:       "2026-07-18T00:00:00Z",
		Licenses:      []assetsdb.License{{Name: "package-license"}},
		SchemaVersion: 1,
		Sources: []assetsdb.Source{
			{
				Name: "source-b", Title: "Source B", Path: "stored/source-b.raw", Type: "curated",
				Licenses: []assetsdb.License{{Name: "CC0-1.0"}}, Tags: []string{"audio"},
			},
			{
				Name: "source-a", Title: "Source A", Path: "archives/custom-a.zip", Type: "curated",
				Licenses: []assetsdb.License{{Name: "CC0-1.0"}}, Tags: []string{"icons"},
			},
		},
		Resources: []assetsdb.Item{
			{
				Name: "z", Path: "images/z.png", ID: "assetsdb:source-a/z.png", Source: "source-a",
				Title: "Silver Coin", Tokens: []string{"silver", "coin"},
			},
			{
				Name: "hit", Path: "audio/hit.ogg", ID: "assetsdb:source-b/hit.ogg", Source: "source-b",
				Title: "Hit Sound", Tokens: []string{"hit", "sound"},
			},
			{
				Name: "a", Path: "images/a.png", ID: "assetsdb:source-a/a.png", Source: "source-a",
				Title: "Gold Coin", Tokens: []string{"coin", "gold"}, Region: &assetsdb.Region{X: 1},
				Provenance: &assetsdb.Provenance{Params: map[string]string{"seed": "1"}},
			},
		},
	}
}

func readTestDB(
	t *testing.T,
	root string,
	dataPackage *assetsdb.DataPackage,
	archives map[string][]byte,
) *assetsdb.DB {
	t.Helper()

	for path, contents := range archives {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o750))
		require.NoError(t, os.WriteFile(fullPath, contents, 0o600))
	}

	index, err := os.Create(filepath.Join(root, "datapackage.json")) // #nosec G304 -- test path.
	require.NoError(t, err)
	require.NoError(t, assetsdb.Encode(index, dataPackage))
	require.NoError(t, index.Close())

	db, err := assetsdb.Read(root)
	require.NoError(t, err)

	return db
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, contents := range files {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(entry, contents)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	return buffer.Bytes()
}

func readAndClose(t *testing.T, reader io.ReadCloser) []byte {
	t.Helper()

	contents, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	return contents
}

func itemByID(t *testing.T, dataPackage *assetsdb.DataPackage, id string) assetsdb.Item {
	t.Helper()

	for _, item := range dataPackage.Resources {
		if item.ID == id {
			return item
		}
	}

	t.Fatalf("item %q not found in test data package", id)
	return assetsdb.Item{}
}

func itemIDs(items []assetsdb.Item) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}

	return ids
}
