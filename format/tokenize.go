package format

import (
	"path"
	"regexp"
	"strings"
)

// bucketDirs is the stoplist of generic type-bucket directory names excluded from path-derived
// search tokens: they correlate 1:1 with an item's Kind/format and add no discriminative value.
var bucketDirs = map[string]struct{}{
	"sprites":  {},
	"png":      {},
	"textures": {},
	"models":   {},
	"gltf":     {},
	"glb":      {},
	"fbx":      {},
	"obj":      {},
	"audio":    {},
	"sfx":      {},
	"music":    {},
	"fonts":    {},
	"source":   {},
	"preview":  {},
}

// delimiterSplitter splits an identifier on the explicit filename delimiters: underscore,
// hyphen, space, and dot.
var delimiterSplitter = regexp.MustCompile(`[_\-\s.]+`)

// wordSplitter finds camelCase words, acronym runs, and digit runs within a delimiter-free
// segment: an uppercase run with an optional lowercase tail (handles both "Walk" and
// acronym-leading "HTTPServer" -> "HTTP", "Server"), otherwise a lowercase run, otherwise a
// digit run.
var wordSplitter = regexp.MustCompile(`[A-Z]+[a-z]*|[a-z]+|[0-9]+`)

// Tokenize splits path into lowercase, order-preserving deduplicated search tokens: the
// filename (minus extension) is split on delimiters and camelCase/letter-digit boundaries, and
// the single immediate parent directory is included too, unless it is a generic bucket
// directory (see bucketDirs).
func Tokenize(p string) []string {
	dir, file := path.Split(p)
	name := strings.TrimSuffix(file, path.Ext(file))

	tokens := splitIdentifier(name)
	if seg := meaningfulParentDir(dir); seg != "" {
		tokens = append(tokens, splitIdentifier(seg)...)
	}

	return dedupeLower(tokens)
}

// meaningfulParentDir returns the immediate parent directory segment of dir, or "" if there is
// none or it is a generic bucket directory.
func meaningfulParentDir(dir string) string {
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return ""
	}

	seg := path.Base(dir)
	if _, stop := bucketDirs[strings.ToLower(seg)]; stop {
		return ""
	}

	return seg
}

// splitIdentifier splits s on delimiters, then splits each resulting part on camelCase and
// letter/digit boundaries.
func splitIdentifier(s string) []string {
	var out []string
	for _, part := range delimiterSplitter.Split(s, -1) {
		out = append(out, wordSplitter.FindAllString(part, -1)...)
	}
	return out
}

// dedupeLower lowercases every token and removes duplicates, keeping the first occurrence of
// each and dropping empty tokens.
func dedupeLower(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		lt := strings.ToLower(t)
		if lt == "" {
			continue
		}
		if _, ok := seen[lt]; ok {
			continue
		}
		seen[lt] = struct{}{}
		out = append(out, lt)
	}
	return out
}
