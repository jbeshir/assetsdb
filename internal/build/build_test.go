package build

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jbeshir/assetsdb"
)

const (
	ingesterA = "fakeA"
	ingesterB = "fakeB"
	packAName = "pack-a"
	packBName = "pack-b"
	spdxCC0   = "CC0-1.0"
)

// fakeIngester is a network-free assetsdb.Ingester stand-in for testing Build.
type fakeIngester struct {
	name  string
	src   assetsdb.Source
	items []assetsdb.Item
	zip   []byte
}

// Name returns f's configured name.
func (f *fakeIngester) Name() string { return f.name }

// Ingest returns f's configured Source, Items, and zip bytes, ignoring its arguments.
func (f *fakeIngester) Ingest(_ context.Context, _ assetsdb.PackSpec) (assetsdb.Source, []assetsdb.Item, []byte, error) {
	return f.src, f.items, f.zip, nil
}

// buildZip returns the bytes of a minimal ZIP archive containing files, for use as fake
// downloaded pack data.
func buildZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()

	ingesters := map[string]assetsdb.Ingester{
		ingesterA: &fakeIngester{
			name: ingesterA,
			src: assetsdb.Source{
				Name:     packAName,
				Title:    "Pack A",
				Path:     "sources/pack-a.zip",
				Licenses: []assetsdb.License{{Name: spdxCC0}},
				Type:     "curated",
			},
			items: []assetsdb.Item{
				{
					Name:   "pack-a-widget",
					Path:   "a.png",
					ID:     "assetsdb:pack-a/a.png",
					Source: packAName,
					Kind:   assetsdb.KindSprite2D,
					Title:  "Alpha Widget",
					Tokens: []string{"alpha", "widget"},
				},
			},
			zip: buildZip(t, map[string][]byte{"a.png": []byte("dataA")}),
		},
		ingesterB: &fakeIngester{
			name: ingesterB,
			src: assetsdb.Source{
				Name:     packBName,
				Title:    "Pack B",
				Path:     "sources/pack-b.zip",
				Licenses: []assetsdb.License{{Name: spdxCC0}},
				Type:     "curated",
			},
			items: []assetsdb.Item{
				{
					Name:   "pack-b-explosion",
					Path:   "b.ogg",
					ID:     "assetsdb:pack-b/b.ogg",
					Source: packBName,
					Kind:   assetsdb.KindAudio,
					Title:  "Explosion Sound",
					Tokens: []string{"explosion", "sound"},
				},
			},
			zip: buildZip(t, map[string][]byte{"b.ogg": []byte("dataB")}),
		},
	}

	specs := []assetsdb.PackSpec{
		{ID: packBName, Name: "Pack B", Source: ingesterB},
		{ID: packAName, Name: "Pack A", Source: ingesterA},
	}

	now := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)

	dp, err := Build(t.Context(), specs, ingesters, dir, now)
	require.NoError(t, err)
	require.Equal(t, "assetsdb", dp.Name)
	require.Equal(t, "2026-07-18T00:00:00Z", dp.Created)
	require.Len(t, dp.Sources, 2)

	require.Len(t, dp.Resources, 2)
	require.Equal(t, "assetsdb:pack-a/a.png", dp.Resources[0].ID)
	require.Equal(t, "assetsdb:pack-b/b.ogg", dp.Resources[1].ID)

	require.FileExists(t, filepath.Join(dir, "datapackage.json"))
	require.FileExists(t, filepath.Join(dir, "sources", "pack-a.zip"))
	require.FileExists(t, filepath.Join(dir, "sources", "pack-b.zip"))
	require.DirExists(t, filepath.Join(dir, "generated"))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotContains(t, entry.Name(), ".datapackage-")
	}

	db, err := assetsdb.Read(dir)
	require.NoError(t, err)

	hits := db.Search("explosion")
	require.Len(t, hits, 1)
	require.Equal(t, "assetsdb:pack-b/b.ogg", hits[0].ID)
}

func TestBuildUnknownSource(t *testing.T) {
	dir := t.TempDir()
	specs := []assetsdb.PackSpec{{ID: "x", Source: "missing"}}

	_, err := Build(t.Context(), specs, map[string]assetsdb.Ingester{}, dir, time.Now())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnknownSource)
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "packs.yaml")
	content := `
packs:
  - id: test-pack
    name: "Test Pack"
    source: kenney
    url: https://kenney.nl/assets/test-pack
    theme: test
    tags: [alpha, beta]
    license: CC0-1.0
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(content), 0o600))

	specs, err := LoadManifest(manifestPath)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	require.Equal(t, "test-pack", specs[0].ID)
	require.Equal(t, "Test Pack", specs[0].Name)
	require.Equal(t, "kenney", specs[0].Source)
	require.Equal(t, []string{"alpha", "beta"}, specs[0].Tags)
	require.Equal(t, spdxCC0, specs[0].License)
}

func TestLoadManifestRealPacksYAML(t *testing.T) {
	specs, err := LoadManifest("../../packs.yaml")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(specs), 10)

	ids := make(map[string]bool, len(specs))
	for _, spec := range specs {
		ids[spec.ID] = true
	}

	require.True(t, ids["kaykit-furniture-bits"])
	require.True(t, ids["kaykit-city-builder-bits"])
}
