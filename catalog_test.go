package assetsdb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	spdxCC0        = "CC0-1.0"
	themeSpace     = "space"
	themeFurniture = "furniture"
	tagShooter     = "shooter"
)

func TestLicenseForSPDX(t *testing.T) {
	require.Equal(t, License{
		Name:  spdxCC0,
		Path:  "https://creativecommons.org/publicdomain/zero/1.0/legalcode",
		Title: "Creative Commons Zero v1.0 Universal",
	}, LicenseForSPDX(spdxCC0))

	require.Equal(t, License{Name: "MIT"}, LicenseForSPDX("MIT"))
	require.Equal(t, License{}, LicenseForSPDX(""))
}

func TestHash(t *testing.T) {
	// Known SHA-256 of the empty input, algorithm-prefixed.
	require.Equal(t,
		"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Hash(nil))
	require.Equal(t,
		"sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
		Hash([]byte("foo")))
}

func TestItemID(t *testing.T) {
	require.Equal(t, "assetsdb:pack/dir/file.png", ItemID("pack", "dir/file.png", ""))
	require.Equal(t, "assetsdb:pack/sheet.png#sprite_01", ItemID("pack", "sheet.png", "sprite_01"))
}

func TestExtNoDot(t *testing.T) {
	require.Equal(t, "png", ExtNoDot("dir/coin.PNG"))
	require.Equal(t, "glb", ExtNoDot("a/b/model.gltf.glb"))
	require.Empty(t, ExtNoDot("dir/README"))
}

func TestSlugify(t *testing.T) {
	require.Equal(t, "a-b", Slugify("a  b"))
	require.Equal(t, "foo-bar", Slugify("--Foo!!  Bar--")) // collapse runs + trim edge hyphens
	require.Equal(t, "keep_dots.and-dashes", Slugify("keep_dots.and-dashes"))
	require.Equal(t, "trimmed", Slugify("  trimmed!!  "))
	require.Equal(t, "chair_a", Slugify("chair_A")) // underscores are valid name chars, kept
}

func TestNameAllocator(t *testing.T) {
	a := NewNameAllocator()
	require.Equal(t, "base", a.Allocate("base"))
	require.Equal(t, "base-2", a.Allocate("base"))
	require.Equal(t, "base-3", a.Allocate("base"))
	require.Equal(t, "other", a.Allocate("other"))
}

func TestTitle(t *testing.T) {
	require.Equal(t, "Chair A", Title("chair_A"))
	require.Equal(t, "Bunny 1 Stand", Title("bunny1_stand"))
	require.Equal(t, "Banner Blue", Title("banner_blue"))
	require.Equal(t, "Space Shooter Enemy Black 1", Title("spaceShooter_enemyBlack1"))

	// Title reuses Tokenize's word splitting: for a bare stem with no duplicate tokens, the
	// title is exactly the capitalized form of the tokens Tokenize produces.
	for _, base := range []string{"spaceShooter_enemyBlack1", "knight_axe_2h", "tile_0043"} {
		want := make([]string, 0)
		for _, tok := range Tokenize(base) {
			want = append(want, capitalizeFirst(tok))
		}
		require.Equal(t, strings.Join(want, " "), Title(base), "base %q", base)
	}
}

func TestCuratedSource(t *testing.T) {
	spec := PackSpec{
		ID:    "kenney-space",
		Name:  "Kenney Space",
		URL:   "https://kenney.nl/assets/space",
		Theme: themeSpace,
		Tags:  []string{tagShooter, "2d"},
	}

	src := CuratedSource(spec, LicenseForSPDX(spdxCC0))
	require.Equal(t, "kenney-space", src.Name)
	require.Equal(t, "Kenney Space", src.Title)
	require.Equal(t, "sources/kenney-space.zip", src.Path)
	require.Equal(t, "curated", src.Type)
	require.Equal(t, "https://kenney.nl/assets/space", src.Origin)
	// Theme folded to the front of the tags.
	require.Equal(t, []string{themeSpace, tagShooter, "2d"}, src.Tags)
	require.Equal(t, []License{LicenseForSPDX(spdxCC0)}, src.Licenses)
}

func TestCuratedSourceThemeAlreadyInTags(t *testing.T) {
	spec := PackSpec{
		ID:    "p",
		Theme: themeFurniture,
		Tags:  []string{themeFurniture, "interior"},
	}
	src := CuratedSource(spec, LicenseForSPDX(spdxCC0))
	// Theme is not duplicated when it already appears in the tags.
	require.Equal(t, []string{themeFurniture, "interior"}, src.Tags)
}

func TestCuratedSourceZeroLicenseOmitted(t *testing.T) {
	spec := PackSpec{ID: "p", Name: "P"}
	src := CuratedSource(spec, License{})
	require.Nil(t, src.Licenses)
}
