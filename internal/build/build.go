// Package build implements the assetsdb manifest loader and the build orchestrator: it fetches
// each curated pack via its format.Ingester, stores the raw archive it returns, and assembles the
// resulting datapackage.json. It contains no ingester-specific or provider-specific logic.
package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/jbeshir/assetsdb/format"
)

// ErrUnknownSource is returned when a PackSpec names a Source with no matching entry in the
// ingesters map passed to Build.
var ErrUnknownSource = errors.New("assetsdb: no ingester for source")

// packageVersion is the semver of the datapackage.json format content Build assembles.
const packageVersion = "1.0.0"

// packageLicense is the package-level default license recorded on every built DataPackage: every
// v1 curated source is CC0-1.0.
var packageLicense = format.License{
	Name:  "CC0-1.0",
	Path:  "https://creativecommons.org/publicdomain/zero/1.0/legalcode",
	Title: "Creative Commons Zero v1.0 Universal",
}

// manifestFile is the top-level shape of a packs manifest YAML file.
type manifestFile struct {
	Packs []manifestPack `yaml:"packs"`
}

// manifestPack is one manifest entry, mapping directly onto format.PackSpec.
type manifestPack struct {
	ID      string   `yaml:"id"`
	Name    string   `yaml:"name"`
	Source  string   `yaml:"source"`
	URL     string   `yaml:"url"`
	Theme   string   `yaml:"theme"`
	Tags    []string `yaml:"tags"`
	License string   `yaml:"license"`
}

// LoadManifest parses the packs manifest YAML file at path into a slice of format.PackSpec.
func LoadManifest(path string) ([]format.PackSpec, error) {
	// #nosec G304 -- path is the caller-supplied manifest file path
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("assetsdb: read manifest %s: %w", path, err)
	}

	var mf manifestFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("assetsdb: parse manifest %s: %w", path, err)
	}

	specs := make([]format.PackSpec, 0, len(mf.Packs))
	for _, p := range mf.Packs {
		specs = append(specs, format.PackSpec{
			ID:      p.ID,
			Name:    p.Name,
			Source:  p.Source,
			URL:     p.URL,
			Theme:   p.Theme,
			Tags:    p.Tags,
			License: p.License,
		})
	}

	return specs, nil
}

// Build fetches every pack in specs via the matching entry in ingesters, stores each pack's raw
// archive under outDir/sources, and writes the assembled datapackage.json to outDir (also
// creating the reserved outDir/generated directory). now is the injectable build clock recorded
// as the package's Created time. It fails fast on the first pack that errors.
func Build(
	ctx context.Context,
	specs []format.PackSpec,
	ingesters map[string]format.Ingester,
	outDir string,
	now time.Time,
) (*format.DataPackage, error) {
	if err := makeOutputDirs(outDir); err != nil {
		return nil, err
	}

	sources, items, err := ingestAll(ctx, specs, ingesters, outDir)
	if err != nil {
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	dp := &format.DataPackage{
		Name:          "assetsdb",
		Title:         "assetsdb asset library",
		Version:       packageVersion,
		Created:       now.UTC().Format(time.RFC3339),
		Licenses:      []format.License{packageLicense},
		SchemaVersion: 1,
		Sources:       sources,
		Resources:     items,
	}

	if err := format.Write(outDir, dp); err != nil {
		return nil, fmt.Errorf("assetsdb: write datapackage: %w", err)
	}

	return dp, nil
}

// makeOutputDirs creates outDir and its sources and generated subdirectories.
func makeOutputDirs(outDir string) error {
	dirs := []string{outDir, filepath.Join(outDir, "sources"), filepath.Join(outDir, "generated")}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return fmt.Errorf("assetsdb: create %s: %w", d, err)
		}
	}
	return nil
}

// ingestAll runs every spec through ingestOne, collecting each pack's Source and Items. It stops
// and wraps the error at the first pack that fails, naming which pack failed.
func ingestAll(
	ctx context.Context,
	specs []format.PackSpec,
	ingesters map[string]format.Ingester,
	outDir string,
) ([]format.Source, []format.Item, error) {
	sources := make([]format.Source, 0, len(specs))
	var items []format.Item

	for _, spec := range specs {
		src, specItems, err := ingestOne(ctx, spec, ingesters, outDir)
		if err != nil {
			return nil, nil, fmt.Errorf("assetsdb: pack %s: %w", spec.ID, err)
		}
		sources = append(sources, src)
		items = append(items, specItems...)
	}

	return sources, items, nil
}

// ingestOne looks up spec's ingester, fetches the pack, and persists its raw archive under
// outDir/sources/<spec.ID>.zip.
func ingestOne(
	ctx context.Context,
	spec format.PackSpec,
	ingesters map[string]format.Ingester,
	outDir string,
) (format.Source, []format.Item, error) {
	in, ok := ingesters[spec.Source]
	if !ok {
		return format.Source{}, nil, fmt.Errorf("%w: %s", ErrUnknownSource, spec.Source)
	}

	src, items, zipData, err := in.Ingest(ctx, spec)
	if err != nil {
		return format.Source{}, nil, fmt.Errorf("ingest: %w", err)
	}

	zipPath := filepath.Join(outDir, "sources", spec.ID+".zip")
	if err := os.WriteFile(zipPath, zipData, 0o600); err != nil {
		return format.Source{}, nil, fmt.Errorf("write %s: %w", zipPath, err)
	}

	return src, items, nil
}
