package format

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		path     string
		wantKind Kind
		wantOK   bool
	}{
		{"Spritesheets/PNG/alienYellow_walk2.png", KindSprite2D, true},
		{"gltf/Skeleton_Warrior.gltf", KindModel3D, true},
		{"SFX/explosion1.ogg", KindAudio, true},
		{"Fonts/Kenney Future.ttf", KindFont, true},
		{"License.txt", "", false},
		{"chair_A.mtl", "", false},
		{"chair_A.bin", "", false},
	}

	for _, c := range cases {
		kind, ok := Classify(c.path)
		require.Equal(t, c.wantOK, ok, "path %q", c.path)
		require.Equal(t, c.wantKind, kind, "path %q", c.path)
	}
}

func TestMediaType(t *testing.T) {
	require.Equal(t, "image/png", MediaType("sprites/coin.png"))
	require.Equal(t, "model/gltf-binary", MediaType("models/chair.GLB"))
	require.Equal(t, "audio/wav", MediaType("sfx/hit.wav"))
	require.Empty(t, MediaType("readme.txt"))
}
