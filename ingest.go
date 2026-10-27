package assetsdb

import "context"

// Ingester is implemented by each curated-pack importer (Kenney, KayKit, ...). It fetches one
// pack described by a PackSpec and returns everything needed to catalog it: the pack's own
// Source metadata, its Items, and the raw archive bytes to persist under sources/<id>.zip.
type Ingester interface {
	// Name identifies this ingester, matched against PackSpec.Source (e.g. "kenney", "kaykit").
	Name() string

	// Ingest fetches the pack and returns its Source metadata, its Items, and the raw ZIP bytes
	// to persist under sources/<id>.zip. Items' Path fields are relative to the ZIP root.
	Ingest(ctx context.Context, spec PackSpec) (src Source, items []Item, zip []byte, err error)
}

// PackSpec is one entry in the packs manifest, describing a single curated asset pack to fetch
// and catalog.
type PackSpec struct {
	// ID is the pack's source id, used as its Source.Name and as sources/<ID>.zip.
	ID string
	// Name is the pack's human-readable title.
	Name string
	// Source selects which Ingester handles this pack (e.g. "kenney", "kaykit").
	Source string
	// URL is the upstream pack page or repository to fetch from.
	URL string
	// Theme is a free-text category used for tagging, e.g. "space", "dungeon".
	Theme string
	// Tags are additional search/category tags carried onto the resulting Source.
	Tags []string
	// License is the SPDX identifier to record for the pack (e.g. "CC0-1.0").
	License string
}
