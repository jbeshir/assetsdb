package build

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/require"

	"github.com/jbeshir/assetsdb"
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
func (f *fakeIngester) Ingest(_ context.Context, spec assetsdb.PackSpec) (assetsdb.Source, []assetsdb.Item, []byte, error) {
	src, items := f.withExtraction(spec)
	return src, items, f.zip, nil
}

func (f *fakeIngester) Reindex(spec assetsdb.PackSpec, _ []byte) (assetsdb.Source, []assetsdb.Item, error) {
	src, items := f.withExtraction(spec)
	return src, items, nil
}

func (f *fakeIngester) withExtraction(spec assetsdb.PackSpec) (assetsdb.Source, []assetsdb.Item) {
	src := f.src
	src.ExtractionPolicy = spec.Extraction
	items := append([]assetsdb.Item(nil), f.items...)
	for i := range items {
		items[i].ExtractionPolicy = spec.Extraction
		items[i].NamingEvidence = assetsdb.NamingEvidence{Kind: "test", Value: items[i].ID}
	}
	return src, items
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
		"fakeA": &fakeIngester{
			name: "fakeA",
			src: assetsdb.Source{
				Name:     "pack-a",
				Title:    "Pack A",
				Path:     "sources/pack-a.zip",
				Licenses: []assetsdb.License{{Name: "CC0-1.0"}},
				Type:     "curated",
			},
			items: []assetsdb.Item{
				{
					Name:   "pack-a-widget",
					Path:   "a.png",
					ID:     "assetsdb:pack-a/a.png",
					Source: "pack-a",
					Kind:   assetsdb.KindSprite2D,
					Title:  "Alpha Widget",
					Tokens: []string{"alpha", "widget"},
				},
			},
			zip: buildZip(t, map[string][]byte{"a.png": []byte("dataA")}),
		},
		"fakeB": &fakeIngester{
			name: "fakeB",
			src: assetsdb.Source{
				Name:     "pack-b",
				Title:    "Pack B",
				Path:     "sources/pack-b.zip",
				Licenses: []assetsdb.License{{Name: "CC0-1.0"}},
				Type:     "curated",
			},
			items: []assetsdb.Item{
				{
					Name:   "pack-b-explosion",
					Path:   "b.ogg",
					ID:     "assetsdb:pack-b/b.ogg",
					Source: "pack-b",
					Kind:   assetsdb.KindAudio,
					Title:  "Explosion Sound",
					Tokens: []string{"explosion", "sound"},
				},
			},
			zip: buildZip(t, map[string][]byte{"b.ogg": []byte("dataB")}),
		},
	}

	specs := []assetsdb.PackSpec{
		{ID: "pack-b", Name: "Pack B", Source: "fakeB", Extraction: assetsdb.ExtractionSemanticFiles},
		{ID: "pack-a", Name: "Pack A", Source: "fakeA", Extraction: assetsdb.ExtractionSemanticFiles},
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
	specs := []assetsdb.PackSpec{{ID: "x", Source: "missing", Extraction: assetsdb.ExtractionSemanticFiles}}

	_, err := Build(t.Context(), specs, map[string]assetsdb.Ingester{}, dir, time.Now())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnknownSource)
}

func TestReindexUsesExistingArchive(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sources"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sources", "pack.zip"), buildZip(t, map[string][]byte{"door.png": []byte("door")}), 0o600))
	in := &fakeIngester{name: "fake", src: assetsdb.Source{Name: "pack"}, items: []assetsdb.Item{{ID: "assetsdb:pack/door.png"}}}
	dp, err := Reindex(t.Context(), []assetsdb.PackSpec{{ID: "pack", Source: "fake", Extraction: assetsdb.ExtractionSemanticFiles}}, map[string]assetsdb.Ingester{"fake": in}, dir, time.Now())
	require.NoError(t, err)
	require.Len(t, dp.Resources, 1)
	require.FileExists(t, filepath.Join(dir, "sources", "pack.zip"))
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
    extraction: semantic-atlas-and-files
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(content), 0o600))

	specs, err := LoadManifest(manifestPath)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	require.Equal(t, "test-pack", specs[0].ID)
	require.Equal(t, "Test Pack", specs[0].Name)
	require.Equal(t, "kenney", specs[0].Source)
	require.Equal(t, []string{"alpha", "beta"}, specs[0].Tags)
	require.Equal(t, "CC0-1.0", specs[0].License)
	require.Equal(t, assetsdb.ExtractionSemanticAtlasFiles, specs[0].Extraction)
}

func TestLoadManifestRejectsMissingAndUnknownExtraction(t *testing.T) {
	for _, extraction := range []string{"", "magic"} {
		t.Run(extraction, func(t *testing.T) {
			manifestPath := filepath.Join(t.TempDir(), "packs.yaml")
			content := "packs:\n  - id: bad\n    extraction: " + extraction + "\n"
			require.NoError(t, os.WriteFile(manifestPath, []byte(content), 0o600))
			_, err := LoadManifest(manifestPath)
			require.ErrorIs(t, err, ErrInvalidExtractionPolicy)
		})
	}
}

func TestBuildAndReindexRejectInvalidExtraction(t *testing.T) {
	specs := []assetsdb.PackSpec{{ID: "bad", Source: "fixture"}}
	_, err := Build(t.Context(), specs, nil, t.TempDir(), time.Now())
	require.ErrorIs(t, err, ErrInvalidExtractionPolicy)
	_, err = Reindex(t.Context(), specs, nil, t.TempDir(), time.Now())
	require.ErrorIs(t, err, ErrInvalidExtractionPolicy)
}

func TestAllExtractionPoliciesAreAccepted(t *testing.T) {
	for _, extraction := range []assetsdb.ExtractionPolicy{
		assetsdb.ExtractionSemanticFiles, assetsdb.ExtractionSemanticAtlasFiles,
		assetsdb.ExtractionSemanticModels, assetsdb.ExtractionSemanticAudio,
		assetsdb.ExtractionSemanticFonts,
	} {
		manifestPath := filepath.Join(t.TempDir(), "packs.yaml")
		content := "packs:\n  - id: good\n    extraction: " + string(extraction) + "\n"
		require.NoError(t, os.WriteFile(manifestPath, []byte(content), 0o600))
		_, err := LoadManifest(manifestPath)
		require.NoError(t, err)
	}
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
	require.Len(t, specs, 177)

	data, err := os.ReadFile("../../removed-packs.yaml")
	require.NoError(t, err)
	var removed struct {
		Removed []struct {
			ID            string `yaml:"id"`
			RemovalReason string `yaml:"removalReason"`
		} `yaml:"removedPacks"`
	}
	require.NoError(t, yaml.Unmarshal(data, &removed))
	require.Len(t, removed.Removed, 44)
	for _, pack := range removed.Removed {
		require.False(t, ids[pack.ID], "active and removed pack %s", pack.ID)
		require.NotEmpty(t, pack.RemovalReason)
	}
	require.Equal(t, 221, len(specs)+len(removed.Removed))
}
