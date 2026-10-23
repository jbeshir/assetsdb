package format

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func testDataPackage() DataPackage {
	return DataPackage{
		Name:    "assetsdb-test",
		Title:   "Test Package",
		Version: "1.0.0",
		Created: "2026-07-18T00:00:00Z",
		Licenses: []License{
			{
				Name:  spdxCC0,
				Path:  "https://creativecommons.org/publicdomain/zero/1.0/legalcode",
				Title: "Creative Commons Zero v1.0 Universal",
			},
		},
		SchemaVersion: 1,
		Sources: []Source{
			{
				Name:        "kenney-test-pack",
				Title:       "Kenney Test Pack",
				Description: "A minimal test fixture pack.",
				Path:        "sources/kenney-test-pack.zip",
				Licenses:    []License{{Name: spdxCC0}},
				Type:        "curated",
				Origin:      "https://kenney.nl/assets/test-pack",
				Tags:        []string{"test"},
			},
		},
		Resources: []Item{
			{
				Name:      "kenney-test-pack-coin-gold",
				Path:      "PNG/HUD/coin_gold.png",
				Format:    "png",
				MediaType: "image/png",
				Bytes:     1351,
				Title:     "Coin Gold",
				ID:        "assetsdb:kenney-test-pack/PNG/HUD/coin_gold.png",
				Source:    "kenney-test-pack",
				Kind:      KindSprite2D,
				Tokens:    Tokenize("PNG/HUD/coin_gold.png"),
			},
		},
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dp := testDataPackage()

	require.NoError(t, Write(dir, &dp))

	db, err := Read(dir)
	require.NoError(t, err)
	require.Equal(t, dp, *db.pkg)
}

func TestWriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	dp := testDataPackage()
	require.NoError(t, Write(dir, &dp))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, datapackageFile, entries[0].Name())
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	dp := testDataPackage()
	require.NoError(t, Write(dir, &dp))
	db, err := Read(dir)
	require.NoError(t, err)

	require.Len(t, db.Search("coin"), 1)
	require.Len(t, db.Search("coin gold"), 1)
	require.Len(t, db.Search("COIN"), 1)
	require.Empty(t, db.Search("nonexistent"))
}

func TestSourceByID(t *testing.T) {
	dir := t.TempDir()
	dp := testDataPackage()
	require.NoError(t, Write(dir, &dp))
	db, err := Read(dir)
	require.NoError(t, err)

	src, ok := db.SourceByID("kenney-test-pack")
	require.True(t, ok)
	require.Equal(t, dp.Sources[0], src)

	_, ok = db.SourceByID("does-not-exist")
	require.False(t, ok)
}

func TestLicenseFor(t *testing.T) {
	dir := t.TempDir()
	dp := testDataPackage()
	require.NoError(t, Write(dir, &dp))
	db, err := Read(dir)
	require.NoError(t, err)

	lic := db.LicenseFor(dp.Resources[0])
	require.Equal(t, dp.Sources[0].Licenses[0], lic)

	orphan := dp.Resources[0]
	orphan.Source = "unregistered-source"
	require.Equal(t, dp.Licenses[0], db.LicenseFor(orphan))
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	dp := testDataPackage()
	require.NoError(t, Write(dir, &dp))

	sourcesDir := filepath.Join(dir, "sources")
	require.NoError(t, os.MkdirAll(sourcesDir, 0o750))
	zipData, err := os.ReadFile(filepath.Join("testdata", "kenney-min.zip"))
	require.NoError(t, err)
	root, err := os.OpenRoot(sourcesDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	require.NoError(t, root.WriteFile("kenney-test-pack.zip", zipData, 0o600))

	db, err := Read(dir)
	require.NoError(t, err)

	rc, err := db.Open(dp.Resources[0])
	require.NoError(t, err)
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.Len(t, data, 1351)

	missing := dp.Resources[0]
	missing.Path = "PNG/HUD/missing.png"
	_, err = db.Open(missing)
	require.ErrorIs(t, err, ErrNotFound)
}
