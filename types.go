// Package assetsdb defines the on-disk shape of an assetsdb database: a Frictionless Data Package
// v2 hybrid JSON descriptor (datapackage.json), read/write access to it, extension-based asset
// classification, filename tokenization for search, and the Ingester interface implemented by
// each curated-pack importer. It contains no network code and no provider-specific logic.
package assetsdb

// Kind enumerates the asset categories assetsdb classifies files into. There is deliberately no
// catch-all "other" kind: files with an unrecognized or ignored extension are skipped by
// Classify rather than bucketed, so the set of kinds stays exhaustive and meaningful.
type Kind string

const (
	// KindSprite2D is a raster or vector 2D image: a sprite, spritesheet, or UI texture.
	KindSprite2D Kind = "sprite2d"
	// KindModel3D is a 3D model or scene file (glTF/GLB, FBX, OBJ, and similar formats).
	KindModel3D Kind = "model3d"
	// KindAudio is a sound effect or music file.
	KindAudio Kind = "audio"
	// KindFont is a font file.
	KindFont Kind = "font"
)

// License is a Frictionless Data Package license entry. At least one of Name or Path should be
// set; Name carries an SPDX identifier (e.g. "CC0-1.0") and Title is always optional.
type License struct {
	Name  string `json:"name,omitempty"`
	Path  string `json:"path,omitempty"`
	Title string `json:"title,omitempty"`
}

// Region is an atlas sub-region within a larger spritesheet image, in pixel coordinates. It is
// set on an Item only when that item is a SubTexture cut from a Kenney-style texture atlas.
type Region struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Provenance is RESERVED for phase-2 locally-generated assets. No v1 code path constructs a
// non-nil Provenance: every curated item leaves Item.Provenance nil. The type is defined now so
// the JSON schema and Go API are stable ahead of that later phase.
type Provenance struct {
	Generator        string            `json:"generator,omitempty"`
	Prompt           string            `json:"prompt,omitempty"`
	Model            string            `json:"model,omitempty"`
	Params           map[string]string `json:"params,omitempty"`
	CreatedAt        string            `json:"createdAt,omitempty"`
	LicenseAssertion string            `json:"licenseAssertion,omitempty"`
}

// Source is one curated or generated asset pack, registered in DataPackage's x_assetsdb:sources.
// It mirrors a DCAT dataset: a named, licensed collection of items whose bytes are stored at Path.
type Source struct {
	Name        string    `json:"name"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Path        string    `json:"path"`
	Licenses    []License `json:"licenses,omitempty"`
	// Type is "curated" or "generated".
	Type string `json:"x_assetsdb:type"`
	// Origin is the upstream URL or repository the pack was fetched from.
	Origin string `json:"x_assetsdb:origin,omitempty"`
	// Tags are theme/category tags carried over from the packs manifest.
	Tags []string `json:"x_assetsdb:tags,omitempty"`
}

// Item is a single catalogued asset file, stored as a Frictionless Data Resource in
// DataPackage.Resources. An Item never carries its own license: callers resolve the effective
// license via DB.LicenseFor, which walks the Frictionless inheritance chain from Source to
// package default.
type Item struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Format    string `json:"format,omitempty"`
	MediaType string `json:"mediatype,omitempty"`
	Bytes     int64  `json:"bytes,omitempty"`
	Hash      string `json:"hash,omitempty"`
	Title     string `json:"title,omitempty"`
	// ID is the canonical stable global id: "assetsdb:<sourceid>/<path>", with a "#<subtexture>"
	// suffix for atlas sub-sprites.
	ID string `json:"x_assetsdb:id"`
	// Source is the owning Source's name, resolvable via DB.SourceByID.
	Source string   `json:"x_assetsdb:source"`
	Kind   Kind     `json:"x_assetsdb:kind"`
	Tokens []string `json:"x_assetsdb:tokens,omitempty"`
	// Region is set only for an atlas sub-sprite; nil for a whole-file item.
	Region *Region `json:"x_assetsdb:region,omitempty"`
	// Provenance is RESERVED for phase-2 generated sources; see the Provenance type doc. Always
	// nil for curated items.
	Provenance *Provenance `json:"x_assetsdb:provenance,omitempty"`
}

// DataPackage is the root of datapackage.json: a Frictionless Data Package v2 descriptor whose
// REQUIRED resources[] holds every catalogued Item, plus a custom x_assetsdb:sources[] registry
// of the asset packs those items belong to.
type DataPackage struct {
	Name     string    `json:"name"`
	Title    string    `json:"title"`
	Version  string    `json:"version"`
	Created  string    `json:"created"`
	Licenses []License `json:"licenses"`
	// SchemaVersion is the on-disk format version; bump on breaking change.
	SchemaVersion int `json:"x_assetsdb:schemaVersion"`
	// Sources is the pack registry that Item.Source values link back into.
	Sources []Source `json:"x_assetsdb:sources"`
	// Resources holds every catalogued Item; REQUIRED by the Frictionless spec.
	Resources []Item `json:"resources"`
}
