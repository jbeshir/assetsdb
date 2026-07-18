package format

import (
	"path/filepath"
	"strings"
)

// kindByExt maps a lowercased file extension (including the leading dot) to the Kind assetsdb
// classifies it as. Extensions absent from this table are ignored: license/readme sidecars,
// atlas/material descriptors, OS cruft, and anything not yet recognized. See
// refs/FINDINGS-classify.md for the extension-by-extension rationale.
var kindByExt = map[string]Kind{
	// sprite2d: raster + vector 2D image formats.
	".png":  KindSprite2D,
	".jpg":  KindSprite2D,
	".jpeg": KindSprite2D,
	".gif":  KindSprite2D,
	".bmp":  KindSprite2D,
	".webp": KindSprite2D,
	".tga":  KindSprite2D,
	".svg":  KindSprite2D,
	".psd":  KindSprite2D,
	".ico":  KindSprite2D,
	".avif": KindSprite2D,

	// model3d: 3D model/scene formats. .bin and .mtl are deliberately absent: they are sidecars
	// of a .gltf/.obj root, not standalone catalog entries.
	".gltf":  KindModel3D,
	".glb":   KindModel3D,
	".fbx":   KindModel3D,
	".obj":   KindModel3D,
	".dae":   KindModel3D,
	".blend": KindModel3D,
	".3ds":   KindModel3D,
	".stl":   KindModel3D,
	".ply":   KindModel3D,
	".x3d":   KindModel3D,

	// audio: sound formats.
	".wav":  KindAudio,
	".ogg":  KindAudio,
	".mp3":  KindAudio,
	".flac": KindAudio,
	".aiff": KindAudio,
	".aif":  KindAudio,
	".m4a":  KindAudio,

	// font: font formats.
	".ttf":   KindFont,
	".otf":   KindFont,
	".woff":  KindFont,
	".woff2": KindFont,
	".eot":   KindFont,
}

// mediaTypeByExt maps a lowercased file extension to a best-effort IANA media type. Only
// extensions with a well-established registered or de-facto type are listed; anything else
// resolves to "" via MediaType.
var mediaTypeByExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".webp": "image/webp",
	".tga":  "image/x-tga",
	".svg":  "image/svg+xml",
	".psd":  "image/vnd.adobe.photoshop",
	".ico":  "image/vnd.microsoft.icon",
	".avif": "image/avif",

	".gltf": "model/gltf+json",
	".glb":  "model/gltf-binary",
	".obj":  "model/obj",
	".dae":  "model/vnd.collada+xml",
	".stl":  "model/stl",
	".x3d":  "model/x3d+xml",

	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".aiff": "audio/aiff",
	".aif":  "audio/aiff",
	".m4a":  "audio/mp4",

	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".eot":   "application/vnd.ms-fontobject",
}

// Classify maps path's file extension to an asset Kind. ok is false for ignored or unrecognized
// extensions (license/readme sidecars, atlas/material descriptors, OS cruft, unknown formats) —
// callers skip those files rather than bucketing them into a catch-all kind.
func Classify(path string) (kind Kind, ok bool) {
	kind, ok = kindByExt[extOf(path)]
	return kind, ok
}

// MediaType returns a best-effort IANA media type for path's extension, or "" if unknown.
func MediaType(path string) string {
	return mediaTypeByExt[extOf(path)]
}

func extOf(path string) string {
	return strings.ToLower(filepath.Ext(path))
}
