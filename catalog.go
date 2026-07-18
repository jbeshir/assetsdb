package assetsdb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// spdxLicenses maps a known SPDX identifier to its full Frictionless license entry. Only the
// license identifiers assetsdb ingests today are listed; any other non-empty id falls back to a
// name-only entry via LicenseForSPDX.
var spdxLicenses = map[string]License{
	"CC0-1.0": {
		Name:  "CC0-1.0",
		Path:  "https://creativecommons.org/publicdomain/zero/1.0/legalcode",
		Title: "Creative Commons Zero v1.0 Universal",
	},
}

// LicenseForSPDX returns the Frictionless license entry for an SPDX identifier. A known id (e.g.
// "CC0-1.0") resolves to its full name/path/title entry; any other non-empty id yields a
// name-only entry so the declared license is still recorded; the empty string yields the zero
// License.
func LicenseForSPDX(spdx string) License {
	if spdx == "" {
		return License{}
	}
	if lic, ok := spdxLicenses[spdx]; ok {
		return lic
	}
	return License{Name: spdx}
}

// Hash returns the content hash recorded on an Item: the SHA-256 of contents, hex-encoded and
// carried with the "sha256:" algorithm prefix Frictionless hashes use.
func Hash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ItemID builds an Item's canonical stable global id: "assetsdb:<sourceID>/<pathInSource>", with
// a "#<sub>" suffix appended when sub is non-empty (used for atlas sub-sprites named by their
// SubTexture). This is the single source of truth for the id layout documented on Item.ID.
func ItemID(sourceID, pathInSource, sub string) string {
	id := "assetsdb:" + sourceID + "/" + pathInSource
	if sub != "" {
		id += "#" + sub
	}
	return id
}

// ExtNoDot returns p's lowercased file extension without its leading dot, or "" if p has none. It
// uses path (not path/filepath) because it operates on forward-slash in-zip paths.
func ExtNoDot(p string) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(p)), ".")
}

// slugInvalid matches runs of characters not permitted in a Frictionless resource name.
var slugInvalid = regexp.MustCompile(`[^a-z0-9._-]+`)

// slugDashRun matches runs of two or more consecutive hyphens.
var slugDashRun = regexp.MustCompile(`-{2,}`)

// Slugify lowercases s, replaces every run of disallowed characters with a single hyphen,
// collapses repeated hyphens, and trims leading and trailing hyphens, yielding a valid
// Frictionless resource name matching ^[a-z0-9._-]+$.
func Slugify(s string) string {
	s = slugInvalid.ReplaceAllString(strings.ToLower(s), "-")
	s = slugDashRun.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// NameAllocator hands out deterministic, unique Frictionless resource names within one pack:
// repeated calls with the same base receive "-2", "-3", ... suffixes on collision.
type NameAllocator struct {
	counts map[string]int
}

// NewNameAllocator returns an empty NameAllocator ready to Allocate names.
func NewNameAllocator() *NameAllocator {
	return &NameAllocator{counts: make(map[string]int)}
}

// Allocate returns base unchanged the first time it is seen, and base suffixed with "-N" for the
// Nth occurrence thereafter (base-2, base-3, ...).
func (a *NameAllocator) Allocate(base string) string {
	a.counts[base]++
	if n := a.counts[base]; n > 1 {
		return fmt.Sprintf("%s-%d", base, n)
	}
	return base
}

// Title humanizes an extension-stripped filename stem into a Title Case string, e.g. "chair_A" ->
// "Chair A". It reuses the same delimiter and word splitting as Tokenize (via splitIdentifier),
// so an item's title and its search tokens are always derived by the same word-boundary rule.
func Title(base string) string {
	words := splitIdentifier(base)
	if len(words) == 0 {
		return base
	}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = capitalizeFirst(w)
	}
	return strings.Join(parts, " ")
}

// capitalizeFirst upper-cases s's first rune, leaving the rest unchanged so acronym runs like
// "HTTP" produced by the word splitter stay upper-case.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// CuratedSource builds the Source metadata for a curated pack fetched from spec and stored at
// sources/<id>.zip. The license is recorded only when non-zero, and spec.Theme is folded to the
// front of spec.Tags (deduplicated if it already appears) so the theme is searchable as a tag.
func CuratedSource(spec PackSpec, license License) Source {
	src := Source{
		Name:   spec.ID,
		Title:  spec.Name,
		Path:   "sources/" + spec.ID + ".zip",
		Type:   "curated",
		Origin: spec.URL,
		Tags:   foldTheme(spec.Theme, spec.Tags),
	}
	if license != (License{}) {
		src.Licenses = []License{license}
	}
	return src
}

// foldTheme returns theme folded to the front of tags, skipping theme if it already appears in
// tags so it is never duplicated. An empty theme returns tags unchanged.
func foldTheme(theme string, tags []string) []string {
	if theme == "" {
		return tags
	}
	out := make([]string, 0, len(tags)+1)
	out = append(out, theme)
	for _, t := range tags {
		if t == theme {
			continue
		}
		out = append(out, t)
	}
	return out
}
