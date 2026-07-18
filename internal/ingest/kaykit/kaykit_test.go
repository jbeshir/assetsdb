package kaykit

import (
	"os"
	"testing"

	"github.com/jbeshir/assetsdb/format"
	"github.com/stretchr/testify/require"
)

const fixturePath = "../../../format/testdata/kaykit-min.zip"

func testSpec() format.PackSpec {
	return format.PackSpec{
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

	byPath := make(map[string]format.Item, len(items))
	for _, it := range items {
		byPath[it.Path] = it
	}

	armchairPath := "KayKit-Furniture-Bits-1.0-main/addons/kaykit_furniture_bits/Assets/gltf/armchair.gltf"
	chairPath := "KayKit-Furniture-Bits-1.0-main/addons/kaykit_furniture_bits/Assets/gltf/chair_A.gltf"
	gltfTexPath := "KayKit-Furniture-Bits-1.0-main/addons/kaykit_furniture_bits/Assets/gltf/furniturebits_texture.png"
	texTexPath := "KayKit-Furniture-Bits-1.0-main/addons/kaykit_furniture_bits/Assets/texture/furniturebits_texture.png"

	armchair, ok := byPath[armchairPath]
	require.True(t, ok, "expected model3d item for armchair.gltf")
	require.Equal(t, format.KindModel3D, armchair.Kind)
	require.Equal(t, "gltf", armchair.Format)
	require.Equal(t, []string{"armchair"}, armchair.Tokens)
	require.Equal(t, "Armchair", armchair.Title)
	require.Equal(t, "assetsdb:kaykit-furniture-bits/"+armchairPath, armchair.ID)
	require.Equal(t, "kaykit-furniture-bits", armchair.Source)

	chair, ok := byPath[chairPath]
	require.True(t, ok, "expected model3d item for chair_A.gltf")
	require.Equal(t, format.KindModel3D, chair.Kind)
	require.Equal(t, "Chair A", chair.Title)

	gltfTex, ok := byPath[gltfTexPath]
	require.True(t, ok, "expected sprite2d item for gltf/furniturebits_texture.png")
	require.Equal(t, format.KindSprite2D, gltfTex.Kind)

	texTex, ok := byPath[texTexPath]
	require.True(t, ok, "expected sprite2d item for texture/furniturebits_texture.png")
	require.Equal(t, format.KindSprite2D, texTex.Kind)

	// Skipped files must not produce items under any path.
	for p := range byPath {
		require.NotContains(t, p, "LICENSE.txt")
		require.NotContains(t, p, ".bin")
	}
}

func TestWalkZip_ModelFormatDedupHasNoCollisions(t *testing.T) {
	spec := testSpec()
	items, err := walkZip(spec, loadFixture(t))
	require.NoError(t, err)

	seen := make(map[string]string)
	for _, it := range items {
		if it.Kind != format.KindModel3D {
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
