package assetsdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSemanticUpstreamName(t *testing.T) {
	for _, name := range []string{"door_wood.png", "GoblinWalk3.png", "laser-impact.wav"} {
		require.True(t, SemanticUpstreamName(name), name)
	}
	for _, name := range []string{"tile_0042.png", "item-12.png", "colored-transparent.png"} {
		require.False(t, SemanticUpstreamName(name), name)
	}
}

func TestSemanticUpstreamNameRequiresContentNotOnlyQualifiers(t *testing.T) {
	for _, name := range []string{"tileBlue_01.png", "elementMetal_3.png", "tileCorner_02.png", "texture_A.png"} {
		require.False(t, SemanticUpstreamName(name), name)
	}
	for _, name := range []string{"doorBlue_01.png", "wallCorner_02.png", "tileTree_03.png", "genericButtonPressed.png"} {
		require.True(t, SemanticUpstreamName(name), name)
	}
}

func TestSemanticUpstreamNameForSpecRejectsPackContextOnly(t *testing.T) {
	spec := PackSpec{ID: "kenney-rune-pack", Name: "Kenney Rune Pack", Theme: "fantasy", Tags: []string{"rune"}}
	require.False(t, SemanticUpstreamNameForSpec(spec, "runeBlack_tile_001.png"))
	require.True(t, SemanticUpstreamNameForSpec(spec, "runeBlack_door_001.png"))
}
