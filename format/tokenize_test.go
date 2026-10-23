package format

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTokenizeWorkedExamples matches the worked examples table in refs/FINDINGS-classify.md
// verbatim: bare filenames (no parent directory), asserting the exact documented tokens.
func TestTokenizeWorkedExamples(t *testing.T) {
	cases := []struct {
		path string
		want []string
	}{
		{"alienYellow_walk2.png", []string{"alien", "yellow", "walk", "2"}},
		{"knight_axe_2h.gltf", []string{"knight", "axe", "2", "h"}},
		{"spaceShooter_enemyBlack1.png", []string{"space", tagShooter, "enemy", "black", "1"}},
		{"tile_0043.png", []string{"tile", "0043"}},
		{"barbarian_Idle.fbx", []string{"barbarian", "idle"}},
	}

	for _, c := range cases {
		require.Equal(t, c.want, Tokenize(c.path), "path %q", c.path)
	}
}

func TestTokenizeIncludesMeaningfulParentDir(t *testing.T) {
	require.Equal(t, []string{"robe", "rogue", "hooded"}, Tokenize("Enemies/Rogue_Hooded/robe.png"))
}

func TestTokenizeExcludesBucketParentDir(t *testing.T) {
	require.Equal(t, []string{"coin", "gold"}, Tokenize("Kenney Pack/PNG/coin_gold.png"))
}

func TestTokenizeDedupesCaseInsensitively(t *testing.T) {
	require.Equal(t, []string{"knight", "axe"}, Tokenize("Knight/knight_axe.png"))
}
