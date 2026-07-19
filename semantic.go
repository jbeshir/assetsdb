package assetsdb

import (
	"path"
	"regexp"
	"strings"
)

var numericName = regexp.MustCompile(`^[0-9]+$`)

var opaqueWords = map[string]bool{
	"asset": true, "assets": true, "element": true, "elements": true,
	"generic": true, "image": true, "images": true, "item": true, "items": true,
	"object": true, "objects": true, "packed": true, "sheet": true, "sprite": true,
	"sprites": true, "spritesheet": true, "texture": true, "textures": true, "tile": true, "tiles": true,
	"tilemap": true, "tileset": true, "transparent": true, "colored": true,
	"colour": true, "color": true, "monochrome": true,
	"hud": true, "scifi": true,
}

// qualifierWords describe appearance, orientation, topology, or state but do not identify the
// depicted thing by themselves. They remain useful in a semantic name ("blue door", "wall
// corner"), but a generic stem such as "tileBlue_01" or "tileCorner_02" must not pass merely
// because it contains one of them.
var qualifierWords = map[string]bool{
	"a": true, "b": true, "c": true, "d": true, "e": true, "f": true, "g": true,
	"h": true, "i": true, "j": true, "k": true, "l": true, "m": true, "n": true,
	"o": true, "p": true, "q": true, "r": true, "s": true, "t": true, "u": true,
	"v": true, "w": true, "x": true, "y": true, "z": true,
	"black": true, "blue": true, "brown": true, "cyan": true, "gray": true,
	"green": true, "grey": true, "magenta": true, "orange": true, "pink": true,
	"purple": true, "red": true, "silver": true, "white": true, "yellow": true,
	"bronze": true, "explosive": true, "glass": true, "gold": true, "metal": true,
	"stone": true, "wood": true, "wooden": true,
	"big": true, "double": true, "flat": true, "high": true, "large": true,
	"low": true, "small": true, "tall": true, "thin": true, "wide": true,
	"bottom": true, "corner": true, "crossing": true, "diagonal": true, "down": true,
	"east": true, "end": true, "horizontal": true, "inner": true, "left": true,
	"north": true, "outer": true, "right": true, "round": true, "slope": true,
	"south": true, "split": true, "square": true, "straight": true, "top": true,
	"transition": true, "up": true, "vertical": true, "west": true,
	"default": true, "fill": true, "outline": true, "pressed": true, "spawn": true,
}

// SemanticUpstreamName reports whether an upstream basename contains a meaningful content term.
func SemanticUpstreamName(name string) bool {
	return semanticUpstreamName(name, nil)
}

// SemanticUpstreamNameForSpec applies SemanticUpstreamName while also treating pack-level title,
// theme, and tag words as context rather than item identity. This prevents a theme-only record
// such as "runeBlack_tile_001" in Rune Pack from passing just because it repeats "rune".
func SemanticUpstreamNameForSpec(spec PackSpec, name string) bool {
	contextWords := make(map[string]bool)
	for _, value := range append([]string{spec.ID, spec.Name, spec.Theme}, spec.Tags...) {
		for _, token := range Tokenize(value) {
			contextWords[strings.ToLower(token)] = true
		}
	}
	return semanticUpstreamName(name, contextWords)
}

func semanticUpstreamName(name string, contextWords map[string]bool) bool {
	stem := strings.TrimSuffix(path.Base(name), path.Ext(name))
	for _, token := range Tokenize(stem) {
		lower := strings.ToLower(token)
		if token != "" && !numericName.MatchString(token) && !opaqueWords[lower] &&
			!qualifierWords[lower] && !contextWords[lower] {
			return true
		}
	}
	return false
}

// PolicyAllowsKind limits specialized policies to the asset kind they promise to identify.
func PolicyAllowsKind(policy ExtractionPolicy, kind Kind) bool {
	switch policy {
	case "": // Programmatic callers predating manifest policy validation use the general policy.
		return true
	case ExtractionSemanticModels:
		return kind == KindModel3D
	case ExtractionSemanticAudio:
		return kind == KindAudio
	case ExtractionSemanticFonts:
		return kind == KindFont
	case ExtractionSemanticFiles, ExtractionSemanticAtlasFiles:
		return true
	default:
		return false
	}
}
