package kaykit

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/jbeshir/assetsdb"
	"github.com/stretchr/testify/require"
)

const fixturePath = "../../../testdata/kaykit-min.zip"

func testSpec() assetsdb.PackSpec {
	return assetsdb.PackSpec{
		ID:      "kaykit-furniture-bits",
		Name:    "KayKit Furniture Bits",
		Source:  "kaykit",
		URL:     "https://github.com/KayKit-Game-Assets/KayKit-Furniture-Bits-1.0",
		Theme:   "furniture",
		Tags:    []string{"furniture", "interior"},
		License: "CC0-1.0",
	}
}

func loadFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	return data
}

func TestWalkZip_EmitsExpectedItems(t *testing.T) {
	spec := testSpec()
	items, err := walkZip(spec, loadFixture(t))
	require.NoError(t, err)

	// The fixture has 2 glTF models (armchair, chair_A) and the same texture PNG duplicated
	// under both a gltf/ and a texture/ directory; their .bin buffers, LICENSE.txt, and the
	// directory entries themselves are all not classifiable/skippable and must not appear.
	require.Len(t, items, 4)

	byPath := make(map[string]assetsdb.Item, len(items))
	for _, it := range items {
		byPath[it.Path] = it
	}

	armchairPath := "KayKit-Furniture-Bits-1.0-main/addons/kaykit_furniture_bits/Assets/gltf/armchair.gltf"
	chairPath := "KayKit-Furniture-Bits-1.0-main/addons/kaykit_furniture_bits/Assets/gltf/chair_A.gltf"
	gltfTexPath := "KayKit-Furniture-Bits-1.0-main/addons/kaykit_furniture_bits/Assets/gltf/furniturebits_texture.png"
	texTexPath := "KayKit-Furniture-Bits-1.0-main/addons/kaykit_furniture_bits/Assets/texture/furniturebits_texture.png"

	armchair, ok := byPath[armchairPath]
	require.True(t, ok, "expected model3d item for armchair.gltf")
	require.Equal(t, assetsdb.KindModel3D, armchair.Kind)
	require.Equal(t, "gltf", armchair.Format)
	require.Equal(t, []string{"armchair"}, armchair.Tokens)
	require.Equal(t, "Armchair", armchair.Title)
	require.Equal(t, "assetsdb:kaykit-furniture-bits/"+armchairPath, armchair.ID)
	require.Equal(t, "kaykit-furniture-bits", armchair.Source)

	chair, ok := byPath[chairPath]
	require.True(t, ok, "expected model3d item for chair_A.gltf")
	require.Equal(t, assetsdb.KindModel3D, chair.Kind)
	require.Equal(t, "Chair A", chair.Title)

	gltfTex, ok := byPath[gltfTexPath]
	require.True(t, ok, "expected sprite2d item for gltf/furniturebits_texture.png")
	require.Equal(t, assetsdb.KindSprite2D, gltfTex.Kind)

	texTex, ok := byPath[texTexPath]
	require.True(t, ok, "expected sprite2d item for texture/furniturebits_texture.png")
	require.Equal(t, assetsdb.KindSprite2D, texTex.Kind)

	// Skipped files must not produce items under any path.
	for p := range byPath {
		require.NotContains(t, p, "LICENSE.txt")
		require.NotContains(t, p, ".bin")
	}
}

// TestWalkZip_DedupsSiblingModelFormats exercises the real KayKit layout the fixture never
// covers: each model format lives in its own sibling directory (fbx/, gltf/, obj/), and the
// binary glTF carries a ".gltf.glb" double extension. Against the old dir+basename assetKey these
// produced a distinct key per format and nothing deduped; this test fails there and passes once
// the grouping key normalizes the format-bucket directory and strips all model extensions.
func TestWalkZip_DedupsSiblingModelFormats(t *testing.T) {
	files := map[string][]byte{
		"pack-main/Assets/fbx/banner_blue.fbx":       []byte("fbx-bytes"),
		"pack-main/Assets/gltf/banner_blue.gltf.bin": []byte("bin-bytes"),
		"pack-main/Assets/gltf/banner_blue.gltf.glb": []byte("glb-bytes"),
		"pack-main/Assets/obj/banner_blue.obj":       []byte("obj-bytes"),
		"pack-main/Assets/obj/banner_blue.mtl":       []byte("mtl-bytes"),
		// A second distinct asset, shipped in all three formats, must also collapse to one .glb.
		"pack-main/Assets/fbx/table_round.fbx":       []byte("fbx-bytes"),
		"pack-main/Assets/gltf/table_round.gltf.glb": []byte("glb-bytes"),
		"pack-main/Assets/obj/table_round.obj":       []byte("obj-bytes"),
		// Same basename under two different top directories must NOT be merged.
		"pack-main/Furniture/gltf/chair.gltf": []byte("chair-furniture"),
		"pack-main/Kitchen/gltf/chair.gltf":   []byte("chair-kitchen"),
	}

	items, err := walkZip(testSpec(), buildTestZip(t, files))
	require.NoError(t, err)

	// banner_blue collapses to exactly one item, the preferred .glb.
	banner := itemsMatchingPath(items, "banner_blue")
	require.Len(t, banner, 1)
	require.Equal(t, "glb", banner[0].Format)
	require.True(t, strings.HasSuffix(banner[0].Path, "gltf/banner_blue.gltf.glb"), "path %q", banner[0].Path)

	// table_round likewise collapses to exactly its .glb.
	table := itemsMatchingPath(items, "table_round")
	require.Len(t, table, 1)
	require.Equal(t, "glb", table[0].Format)

	// The two chair assets in different top directories are NOT merged.
	chairs := itemsMatchingPath(items, "chair")
	require.Len(t, chairs, 2)
	require.NotEqual(t, assetKey(chairs[0].Path), assetKey(chairs[1].Path))

	// .bin and .mtl sidecars are excluded entirely.
	for _, it := range items {
		require.NotContains(t, it.Path, ".bin")
		require.NotContains(t, it.Path, ".mtl")
	}

	// The surviving banner_blue item leaks no format token or word from the .gltf.glb double
	// extension.
	require.NotContains(t, banner[0].Tokens, "gltf")
	require.NotContains(t, banner[0].Tokens, "glb")
	require.Equal(t, "Banner Blue", banner[0].Title)
}

// itemsMatchingPath returns the items whose Path contains sub.
func itemsMatchingPath(items []assetsdb.Item, sub string) []assetsdb.Item {
	var out []assetsdb.Item
	for _, it := range items {
		if strings.Contains(it.Path, sub) {
			out = append(out, it)
		}
	}
	return out
}

// buildTestZip returns the bytes of an in-memory ZIP containing files, for synthesizing archives
// with layouts the committed fixture does not cover.
func buildTestZip(t *testing.T, files map[string][]byte) []byte {
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

func TestWalkZip_ModelFormatDedupHasNoCollisions(t *testing.T) {
	spec := testSpec()
	items, err := walkZip(spec, loadFixture(t))
	require.NoError(t, err)

	seen := make(map[string]string)
	for _, it := range items {
		if it.Kind != assetsdb.KindModel3D {
			continue
		}
		key := assetKey(it.Path)
		if prior, ok := seen[key]; ok {
			t.Fatalf("dedup failed: both %q and %q resolved to asset key %q", prior, it.Path, key)
		}
		seen[key] = it.Path
	}
}

func TestWalkZip_IDsUniqueAndPrefixed(t *testing.T) {
	spec := testSpec()
	items, err := walkZip(spec, loadFixture(t))
	require.NoError(t, err)

	seen := make(map[string]bool, len(items))
	for _, it := range items {
		require.False(t, seen[it.ID], "duplicate id %q", it.ID)
		seen[it.ID] = true
		require.Regexp(t, "^assetsdb:kaykit-furniture-bits/", it.ID)
	}
}

func TestWalkZip_ItemsSortedByID(t *testing.T) {
	spec := testSpec()
	items, err := walkZip(spec, loadFixture(t))
	require.NoError(t, err)

	for i := 1; i < len(items); i++ {
		require.Less(t, items[i-1].ID, items[i].ID, "items must be sorted ascending by id")
	}
}

func TestWalkZip_NameSlugsAreUniqueOnCollision(t *testing.T) {
	spec := testSpec()
	items, err := walkZip(spec, loadFixture(t))
	require.NoError(t, err)

	names := make(map[string]bool, len(items))
	for _, it := range items {
		require.False(t, names[it.Name], "duplicate frictionless name %q", it.Name)
		names[it.Name] = true
		require.Regexp(t, "^[a-z0-9._-]+$", it.Name)
	}

	// The two furniturebits_texture.png files (in different dirs) share a basename, so the
	// second one assigned must have collided and received a "-2" suffix.
	require.Contains(t, names, "kaykit-furniture-bits-furniturebits_texture")
	require.Contains(t, names, "kaykit-furniture-bits-furniturebits_texture-2")
}

func TestWalkZip_InvalidZipReturnsError(t *testing.T) {
	_, err := walkZip(testSpec(), []byte("not a zip"))
	require.Error(t, err)
}

func TestParseOwnerRepo(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "full github URL",
			input:     "https://github.com/KayKit-Game-Assets/KayKit-Dungeon-Remastered-1.0",
			wantOwner: "KayKit-Game-Assets",
			wantRepo:  "KayKit-Dungeon-Remastered-1.0",
		},
		{
			name:      "full github URL with trailing slash",
			input:     "https://github.com/KayKit-Game-Assets/KayKit-Dungeon-Remastered-1.0/",
			wantOwner: "KayKit-Game-Assets",
			wantRepo:  "KayKit-Dungeon-Remastered-1.0",
		},
		{
			name:      "owner/repo string",
			input:     "SomeOwner/some-repo",
			wantOwner: "SomeOwner",
			wantRepo:  "some-repo",
		},
		{
			name:      "bare repo defaults owner",
			input:     "KayKit-Furniture-Bits-1.0",
			wantOwner: defaultOwner,
			wantRepo:  "KayKit-Furniture-Bits-1.0",
		},
		{
			name:    "empty string is invalid",
			input:   "",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := parseOwnerRepo(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidSpecURL)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantOwner, owner)
			require.Equal(t, tc.wantRepo, repo)
		})
	}
}
