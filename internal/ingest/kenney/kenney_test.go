package kenney

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jbeshir/assetsdb"
	"github.com/stretchr/testify/require"
)

const fixturePath = "../../../testdata/kenney-min.zip"

// atlasImageEntry is the path the fixture's spritesheet_jumper.xml resolves its imagePath to
// ("Spritesheets/" + imagePath="sprites.png"). The minimal fixture omits the actual atlas image
// to save space (see fixtures/FIXTURES.md), so tests that exercise the atlas sub-sprite path add
// a stub entry at this path on top of the real fixture bytes.
const atlasImageEntry = "Spritesheets/sprites.png"

func TestWalkZip(t *testing.T) {
	data := addStubEntry(t, readFixture(t), atlasImageEntry, []byte("stub-atlas-pixels"))

	spec := assetsdb.PackSpec{
		ID:         "kenney-jumper-pack",
		Name:       "Jumper Pack",
		Tags:       []string{"platformer"},
		Extraction: assetsdb.ExtractionSemanticAtlasFiles,
	}

	items, err := walkZip(spec, data)
	require.NoError(t, err)

	// 7 individually-shipped PNGs (fixtures/FIXTURES.md) + 99 canonical semantic atlas
	// SubTextures. Of the atlas's 117 records, 12 generic tile+qualifier names are rejected and
	// 6 records duplicated by standalone named files are omitted.
	atlas := parseFixtureAtlas(t, data)
	require.Len(t, atlas.SubTextures, 117)
	require.Len(t, items, 7+99)

	var regionItems, wholeFileItems int
	seenIDs := make(map[string]struct{}, len(items))
	seenNames := make(map[string]struct{}, len(items))

	for _, it := range items {
		require.Contains(t, []assetsdb.Kind{assetsdb.KindSprite2D, assetsdb.KindModel3D, assetsdb.KindAudio, assetsdb.KindFont}, it.Kind)
		require.Truef(t, strings.HasPrefix(it.ID, "assetsdb:kenney-jumper-pack/"), "id %q missing source prefix", it.ID)
		require.Equal(t, spec.ID, it.Source)
		require.Nil(t, it.Provenance)
		require.Equal(t, spec.Extraction, it.ExtractionPolicy)
		require.NotEmpty(t, it.NamingEvidence.Value)

		_, dup := seenIDs[it.ID]
		require.False(t, dup, "duplicate id %q", it.ID)
		seenIDs[it.ID] = struct{}{}

		_, dup = seenNames[it.Name]
		require.False(t, dup, "duplicate name %q", it.Name)
		seenNames[it.Name] = struct{}{}

		if it.Region != nil {
			regionItems++
			continue
		}
		wholeFileItems++
	}

	require.Equal(t, 99, regionItems)
	require.Equal(t, 7, wholeFileItems)
	require.True(t, sort.SliceIsSorted(items, func(i, j int) bool { return items[i].ID < items[j].ID }))

	bunny := findItem(t, items, "assetsdb:kenney-jumper-pack/Spritesheets/sprites.png#bunny1_hurt.png")
	require.Equal(t, assetsdb.KindSprite2D, bunny.Kind)
	require.Equal(t, []string{"bunny", "1", "hurt"}, bunny.Tokens)
	require.Equal(t, "Bunny 1 Hurt", bunny.Title)
	require.Equal(t, &assetsdb.Region{X: 382, Y: 946, Width: 150, Height: 174}, bunny.Region)
	require.Zero(t, bunny.Bytes)
	require.Empty(t, bunny.Hash)

	coin := findItem(t, items, "assetsdb:kenney-jumper-pack/PNG/HUD/coin_gold.png")
	require.Equal(t, assetsdb.KindSprite2D, coin.Kind)
	require.Equal(t, []string{"coin", "gold", "hud"}, coin.Tokens)
	require.Equal(t, "Coin Gold", coin.Title)
	require.Nil(t, coin.Region)
	require.Equal(t, int64(1351), coin.Bytes)
	require.Equal(t, "sha256:", coin.Hash[:7])
	require.Equal(t, "png", coin.Format)
	require.Equal(t, "image/png", coin.MediaType)

	for id := range seenIDs {
		require.NotContains(t, id, "License.txt")
		require.NotContains(t, id, ".xml")
	}
}

func TestWalkZip_WithoutAtlasImage(t *testing.T) {
	spec := assetsdb.PackSpec{ID: "kenney-jumper-pack", Name: "Jumper Pack", Extraction: assetsdb.ExtractionSemanticAtlasFiles}

	items, err := walkZip(spec, readFixture(t))
	require.NoError(t, err)

	require.Len(t, items, 7)
	for _, it := range items {
		require.Nil(t, it.Region)
	}
}

func TestWalkZip_SemanticFilesDoesNotExpandAtlas(t *testing.T) {
	data := addStubEntry(t, readFixture(t), atlasImageEntry, []byte("stub-atlas-pixels"))
	spec := assetsdb.PackSpec{ID: "kenney-jumper-pack", Name: "Jumper Pack", Extraction: assetsdb.ExtractionSemanticFiles}

	items, err := walkZip(spec, data)
	require.NoError(t, err)
	require.Len(t, items, 7)
	for _, item := range items {
		require.Nil(t, item.Region)
		require.Equal(t, "file-basename", item.NamingEvidence.Kind)
	}
}

func TestDeduplicateRepresentations(t *testing.T) {
	items := []assetsdb.Item{
		{ID: "default", Path: "PNG/Default/door.png", Hash: "sha256:door", NamingEvidence: assetsdb.NamingEvidence{Value: "door"}},
		{ID: "double", Path: "PNG/Double/door.png", Hash: "sha256:door-2x", NamingEvidence: assetsdb.NamingEvidence{Value: "door"}},
		{ID: "copy", Path: "PNG/Other/door-copy.png", Hash: "sha256:door", NamingEvidence: assetsdb.NamingEvidence{Value: "door-copy"}},
		{ID: "atlas", Path: "Spritesheet/sheet.png", Region: &assetsdb.Region{}, NamingEvidence: assetsdb.NamingEvidence{Value: "door.png"}},
		{ID: "window-atlas", Path: "Spritesheet/sheet.png", Region: &assetsdb.Region{}, NamingEvidence: assetsdb.NamingEvidence{Value: "window"}},
		{ID: "window-retina", Path: "Spritesheet/sheet@2.png", Region: &assetsdb.Region{}, NamingEvidence: assetsdb.NamingEvidence{Value: "window"}},
	}

	got := deduplicateRepresentations(items)
	require.Equal(t, []string{"default", "window-atlas"}, []string{got[0].ID, got[1].ID})
}

func TestSlugFromURL(t *testing.T) {
	slug, err := slugFromURL("https://kenney.nl/assets/1-bit-pack")
	require.NoError(t, err)
	require.Equal(t, "1-bit-pack", slug)

	_, err = slugFromURL("https://example.com/assets/1-bit-pack")
	require.ErrorIs(t, err, ErrInvalidPackURL)
}

func TestExtractZipURL(t *testing.T) {
	// Adapted from the #donate-text fragment documented in refs/FINDINGS-kenney.md.
	const page = `
<div id='inline-download' class='lity-pop lity-hide text-center'>
  <p id='donate-link' class='mobile-text-center'>
    <a href='https://kenney.nl/donate' onclick='initiateDonate()' target='_blank' class='button'>Donate</a>
  </p>
  <p class='margin-top bold'>
    <a id='donate-text'
       href='https://kenney.nl/media/pages/assets/1-bit-pack/aa867a1f37-1677578516/kenney_1-bit-pack.zip'
       onclick='initiateDownload()'
       data-lity-close>Continue without donating...</a>
  </p>
</div>`

	url, err := extractZipURL(page, "1-bit-pack")
	require.NoError(t, err)
	require.Equal(t, "https://kenney.nl/media/pages/assets/1-bit-pack/aa867a1f37-1677578516/kenney_1-bit-pack.zip", url)
}

func TestExtractZipURL_Fallback(t *testing.T) {
	const page = `<a href="https://cdn.example.com/download/1-bit-pack-bundle.zip">Continue without donating...</a>`

	url, err := extractZipURL(page, "1-bit-pack")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.com/download/1-bit-pack-bundle.zip", url)
}

func TestExtractZipURL_NotFound(t *testing.T) {
	_, err := extractZipURL(`<a href="https://kenney.nl/donate">Donate</a>`, "1-bit-pack")
	require.ErrorIs(t, err, ErrZipURLNotFound)
}

func readFixture(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Clean(fixturePath))
	require.NoError(t, err)

	return data
}

// addStubEntry rebuilds a zip archive from data with one additional stub entry appended, so
// tests can exercise atlas-image handling against a fixture that omits the (large) real image.
func addStubEntry(t *testing.T, data []byte, name string, contents []byte) []byte {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}

		w, cerr := zw.Create(f.Name)
		require.NoError(t, cerr)

		rc, oerr := f.Open()
		require.NoError(t, oerr)

		_, cerr = io.Copy(w, rc) // #nosec G110 -- test fixture zip from trusted testdata, not attacker-controlled
		require.NoError(t, cerr)
		require.NoError(t, rc.Close())
	}

	w, err := zw.Create(name)
	require.NoError(t, err)
	_, err = w.Write(contents)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

func parseFixtureAtlas(t *testing.T, data []byte) textureAtlas {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	for _, f := range zr.File {
		if f.Name != "Spritesheets/spritesheet_jumper.xml" {
			continue
		}

		atlas, aerr := readAtlas(f)
		require.NoError(t, aerr)

		return atlas
	}

	t.Fatal("fixture missing Spritesheets/spritesheet_jumper.xml")
	return textureAtlas{}
}

func findItem(t *testing.T, items []assetsdb.Item, id string) assetsdb.Item {
	t.Helper()

	for _, it := range items {
		if it.ID == id {
			return it
		}
	}

	t.Fatalf("item %q not found", id)
	return assetsdb.Item{}
}
