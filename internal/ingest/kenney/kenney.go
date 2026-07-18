// Package kenney implements format.Ingester for Kenney.nl asset packs: resolving a pack page
// URL to its download ZIP, and walking that ZIP into classified, tokenized format.Item values,
// including per-sprite items cut from Sparrow/Starling-style TextureAtlas spritesheets.
package kenney

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/jbeshir/assetsdb/format"
)

// ErrInvalidPackURL is returned when a PackSpec's URL does not match the expected
// https://kenney.nl/assets/<slug> shape.
var ErrInvalidPackURL = errors.New("kenney: pack url does not match https://kenney.nl/assets/<slug>")

// ErrZipURLNotFound is returned when the pack page's HTML does not contain a recognizable ZIP
// download link.
var ErrZipURLNotFound = errors.New("kenney: zip download url not found on pack page")

// ErrUnexpectedStatus is returned when an HTTP fetch responds with a non-200 status code.
var ErrUnexpectedStatus = errors.New("kenney: unexpected http status")

// defaultUserAgent identifies this ingester to kenney.nl.
const defaultUserAgent = "assetsdb-kenney-ingester/1.0 (+https://github.com/jbeshir/assetsdb)"

// kenneyLicense is the license every Kenney pack is distributed under, confirmed by reading the
// License.txt bundled in every downloaded pack (see refs/FINDINGS-kenney.md). It is hard-coded
// rather than parsed per download.
var kenneyLicense = format.License{
	Name:  "CC0-1.0",
	Path:  "https://creativecommons.org/publicdomain/zero/1.0/legalcode",
	Title: "Creative Commons Zero v1.0 Universal",
}

// packURLPattern matches a Kenney pack page URL and captures its slug.
var packURLPattern = regexp.MustCompile(`^https?://kenney\.nl/assets/([a-z0-9-]+)/?$`)

// Ingester implements format.Ingester for Kenney.nl curated asset packs.
type Ingester struct {
	// Client performs HTTP requests. A nil Client uses http.DefaultClient.
	Client *http.Client
}

// New returns an Ingester that fetches pack pages and ZIPs with http.DefaultClient.
func New() *Ingester {
	return &Ingester{}
}

// Name returns "kenney", matched against PackSpec.Source.
func (in *Ingester) Name() string {
	return "kenney"
}

// Ingest fetches the pack page at spec.URL, resolves and downloads its ZIP, and walks the ZIP
// into the pack's Source metadata and Items. The returned zip bytes are the raw, unmodified
// downloaded archive.
func (in *Ingester) Ingest(ctx context.Context, spec format.PackSpec) (format.Source, []format.Item, []byte, error) {
	slug, err := slugFromURL(spec.URL)
	if err != nil {
		return format.Source{}, nil, nil, err
	}

	page, err := in.fetch(ctx, spec.URL)
	if err != nil {
		return format.Source{}, nil, nil, fmt.Errorf("kenney: fetch pack page %s: %w", spec.URL, err)
	}

	zipURL, err := extractZipURL(string(page), slug)
	if err != nil {
		return format.Source{}, nil, nil, err
	}

	data, err := in.fetch(ctx, zipURL)
	if err != nil {
		return format.Source{}, nil, nil, fmt.Errorf("kenney: fetch zip %s: %w", zipURL, err)
	}

	items, err := walkZip(spec, data)
	if err != nil {
		return format.Source{}, nil, nil, fmt.Errorf("kenney: walk zip for %s: %w", spec.ID, err)
	}

	src := format.Source{
		Name:     spec.ID,
		Title:    spec.Name,
		Path:     "sources/" + spec.ID + ".zip",
		Licenses: []format.License{kenneyLicense},
		Type:     "curated",
		Origin:   spec.URL,
		Tags:     spec.Tags,
	}

	return src, items, data, nil
}

// fetch performs a context-aware GET against url and returns the response body, failing on any
// non-200 status.
func (in *Ingester) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("kenney: build request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := in.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("kenney: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kenney: status %d: %w", resp.StatusCode, ErrUnexpectedStatus)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kenney: read body: %w", err)
	}

	return data, nil
}

// httpClient returns in.Client, defaulting to http.DefaultClient.
func (in *Ingester) httpClient() *http.Client {
	if in.Client != nil {
		return in.Client
	}
	return http.DefaultClient
}

// slugFromURL extracts the pack slug from a https://kenney.nl/assets/<slug> pack page URL.
func slugFromURL(rawURL string) (string, error) {
	m := packURLPattern.FindStringSubmatch(rawURL)
	if m == nil {
		return "", fmt.Errorf("kenney: %w: %s", ErrInvalidPackURL, rawURL)
	}
	return m[1], nil
}

// extractZipURL finds the pack ZIP download link within a pack page's HTML: first the
// slug-scoped kenney.nl/media/pages/assets/<slug>/*.zip pattern, falling back to any href
// containing the slug and ending in .zip.
func extractZipURL(html, slug string) (string, error) {
	primary := regexp.MustCompile(
		`href=['"](https?://kenney\.nl/media/pages/assets/` + regexp.QuoteMeta(slug) + `/[^'"]+\.zip)['"]`)
	if m := primary.FindStringSubmatch(html); m != nil {
		return m[1], nil
	}

	fallback := regexp.MustCompile(`href=['"]([^'"]*` + regexp.QuoteMeta(slug) + `[^'"]*\.zip)['"]`)
	if m := fallback.FindStringSubmatch(html); m != nil {
		return m[1], nil
	}

	return "", fmt.Errorf("kenney: %w: slug %q", ErrZipURLNotFound, slug)
}

// textureAtlas is a Sparrow/Starling-style TextureAtlas XML descriptor, as shipped alongside a
// combined Kenney spritesheet.
type textureAtlas struct {
	XMLName     xml.Name     `xml:"TextureAtlas"`
	ImagePath   string       `xml:"imagePath,attr"`
	SubTextures []subTexture `xml:"SubTexture"`
}

// subTexture is one sprite's pixel region within its parent TextureAtlas image.
type subTexture struct {
	Name   string `xml:"name,attr"`
	X      int    `xml:"x,attr"`
	Y      int    `xml:"y,attr"`
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
}

// walkZip walks a downloaded Kenney pack ZIP into its catalog Items. It is pure and
// network-free: every byte it inspects comes from data.
func walkZip(spec format.PackSpec, data []byte) ([]format.Item, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("kenney: open zip: %w", err)
	}

	atlases, err := parseAtlases(zr)
	if err != nil {
		return nil, err
	}

	names := newNameAllocator()
	var items []format.Item

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}

		if subs, ok := atlases[f.Name]; ok {
			items = append(items, atlasItems(spec, f.Name, subs, names)...)
			continue
		}

		item, ok, ferr := fileItem(spec, f, names)
		if ferr != nil {
			return nil, ferr
		}
		if ok {
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	return items, nil
}

// parseAtlases finds every *.xml TextureAtlas descriptor in zr and returns a map from each
// atlas's resolved image path (imagePath resolved relative to the XML's own directory) to its
// SubTextures.
func parseAtlases(zr *zip.Reader) (map[string][]subTexture, error) {
	atlases := make(map[string][]subTexture)

	for _, f := range zr.File {
		if strings.ToLower(path.Ext(f.Name)) != ".xml" {
			continue
		}

		atlas, err := readAtlas(f)
		if err != nil {
			return nil, err
		}
		if atlas.ImagePath == "" {
			continue
		}

		imgPath := path.Join(path.Dir(f.Name), atlas.ImagePath)
		atlases[imgPath] = atlas.SubTextures
	}

	return atlases, nil
}

// readAtlas decodes f as a textureAtlas XML descriptor.
func readAtlas(f *zip.File) (textureAtlas, error) {
	rc, err := f.Open()
	if err != nil {
		return textureAtlas{}, fmt.Errorf("kenney: open %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	var atlas textureAtlas
	if err := xml.NewDecoder(rc).Decode(&atlas); err != nil {
		return textureAtlas{}, fmt.Errorf("kenney: parse atlas %s: %w", f.Name, err)
	}

	return atlas, nil
}

// atlasItems builds one Item per SubTexture cut from the atlas image at atlasPath.
func atlasItems(spec format.PackSpec, atlasPath string, subs []subTexture, names *nameAllocator) []format.Item {
	items := make([]format.Item, 0, len(subs))

	for _, sub := range subs {
		tokens := format.Tokenize(sub.Name)
		base := slugify(spec.ID + "-" + strings.TrimSuffix(sub.Name, path.Ext(sub.Name)))

		items = append(items, format.Item{
			Name:      names.allocate(base),
			Path:      atlasPath,
			Format:    extNoDot(atlasPath),
			MediaType: format.MediaType(atlasPath),
			Title:     humanize(tokens),
			ID:        fmt.Sprintf("assetsdb:%s/%s#%s", spec.ID, atlasPath, sub.Name),
			Source:    spec.ID,
			Kind:      format.KindSprite2D,
			Tokens:    tokens,
			Region:    &format.Region{X: sub.X, Y: sub.Y, Width: sub.Width, Height: sub.Height},
		})
	}

	return items
}

// fileItem builds the Item for a non-atlas zip entry f, or ok=false if format.Classify skips
// its extension.
func fileItem(spec format.PackSpec, f *zip.File, names *nameAllocator) (format.Item, bool, error) {
	kind, ok := format.Classify(f.Name)
	if !ok {
		return format.Item{}, false, nil
	}

	size, hash, err := hashEntry(f)
	if err != nil {
		return format.Item{}, false, err
	}

	tokens := format.Tokenize(f.Name)
	base := slugify(spec.ID + "-" + strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name)))

	item := format.Item{
		Name:      names.allocate(base),
		Path:      f.Name,
		Format:    extNoDot(f.Name),
		MediaType: format.MediaType(f.Name),
		Bytes:     size,
		Hash:      hash,
		Title:     humanize(tokens),
		ID:        fmt.Sprintf("assetsdb:%s/%s", spec.ID, f.Name),
		Source:    spec.ID,
		Kind:      kind,
		Tokens:    tokens,
	}

	return item, true, nil
}

// hashEntry reads f's contents and returns its size and a "sha256:"-prefixed hex digest.
func hashEntry(f *zip.File) (int64, string, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, "", fmt.Errorf("kenney: open %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return 0, "", fmt.Errorf("kenney: read %s: %w", f.Name, err)
	}

	sum := sha256.Sum256(data)
	return int64(len(data)), "sha256:" + hex.EncodeToString(sum[:]), nil
}

// extNoDot returns p's lowercased file extension without its leading dot.
func extNoDot(p string) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(p)), ".")
}

// humanize joins tokens into a human-readable, capitalized title.
func humanize(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = capitalize(t)
	}
	return strings.Join(parts, " ")
}

// capitalize upper-cases s's first rune, leaving the rest unchanged.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// slugInvalid matches runs of characters not allowed in a Frictionless resource name.
var slugInvalid = regexp.MustCompile(`[^a-z0-9._-]+`)

// dashRun matches runs of two or more consecutive hyphens.
var dashRun = regexp.MustCompile(`-{2,}`)

// slugify lowercases s, replaces every run of disallowed characters with a single hyphen,
// collapses repeated hyphens, and trims leading/trailing hyphens.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugInvalid.ReplaceAllString(s, "-")
	s = dashRun.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// nameAllocator hands out deterministic, unique Frictionless resource names for a pack: repeated
// calls with the same base get "-2", "-3", ... suffixes.
type nameAllocator struct {
	counts map[string]int
}

// newNameAllocator returns an empty nameAllocator.
func newNameAllocator() *nameAllocator {
	return &nameAllocator{counts: make(map[string]int)}
}

// allocate returns base unchanged the first time it is seen, and base suffixed with "-N" for the
// Nth collision thereafter.
func (a *nameAllocator) allocate(base string) string {
	a.counts[base]++
	if n := a.counts[base]; n > 1 {
		return fmt.Sprintf("%s-%d", base, n)
	}
	return base
}
