// Package kenney implements assetsdb.Ingester for Kenney.nl asset packs: resolving a pack page
// URL to its download ZIP, and walking that ZIP into classified, tokenized assetsdb.Item values,
// including per-sprite items cut from Sparrow/Starling-style TextureAtlas spritesheets.
package kenney

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jbeshir/assetsdb"
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

// defaultHTTPClient is the fallback HTTP client used when Ingester.Client is nil. It carries a
// timeout so a stalled kenney.nl connection cannot hang a build indefinitely. Every Kenney pack is
// confirmed CC0-1.0 (License.txt in each pack; see refs/FINDINGS-kenney.md); the license itself
// flows through from the manifest via assetsdb.LicenseForSPDX.
var defaultHTTPClient = &http.Client{Timeout: 60 * time.Second}

// packURLPattern matches a Kenney pack page URL and captures its slug.
var packURLPattern = regexp.MustCompile(`^https?://kenney\.nl/assets/([a-z0-9-]+)/?$`)

// Ingester implements assetsdb.Ingester for Kenney.nl curated asset packs.
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
func (in *Ingester) Ingest(
	ctx context.Context, spec assetsdb.PackSpec,
) (assetsdb.Source, []assetsdb.Item, []byte, error) {
	slug, err := slugFromURL(spec.URL)
	if err != nil {
		return assetsdb.Source{}, nil, nil, err
	}

	page, err := in.fetch(ctx, spec.URL)
	if err != nil {
		return assetsdb.Source{}, nil, nil, fmt.Errorf("kenney: fetch pack page %s: %w", spec.URL, err)
	}

	zipURL, err := extractZipURL(string(page), slug)
	if err != nil {
		return assetsdb.Source{}, nil, nil, err
	}

	data, err := in.fetch(ctx, zipURL)
	if err != nil {
		return assetsdb.Source{}, nil, nil, fmt.Errorf("kenney: fetch zip %s: %w", zipURL, err)
	}

	items, err := walkZip(spec, data)
	if err != nil {
		return assetsdb.Source{}, nil, nil, fmt.Errorf("kenney: walk zip for %s: %w", spec.ID, err)
	}

	src := assetsdb.CuratedSource(spec, assetsdb.LicenseForSPDX(spec.License))

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

// httpClient returns in.Client, defaulting to defaultHTTPClient.
func (in *Ingester) httpClient() *http.Client {
	if in.Client != nil {
		return in.Client
	}
	return defaultHTTPClient
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
func walkZip(spec assetsdb.PackSpec, data []byte) ([]assetsdb.Item, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("kenney: open zip: %w", err)
	}

	atlases, err := parseAtlases(zr)
	if err != nil {
		return nil, err
	}

	names := assetsdb.NewNameAllocator()
	var items []assetsdb.Item

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
func atlasItems(
	spec assetsdb.PackSpec, atlasPath string, subs []subTexture, names *assetsdb.NameAllocator,
) []assetsdb.Item {
	items := make([]assetsdb.Item, 0, len(subs))

	for _, sub := range subs {
		stem := strings.TrimSuffix(sub.Name, path.Ext(sub.Name))

		items = append(items, assetsdb.Item{
			Name:      names.Allocate(assetsdb.Slugify(spec.ID + "-" + stem)),
			Path:      atlasPath,
			Format:    assetsdb.ExtNoDot(atlasPath),
			MediaType: assetsdb.MediaType(atlasPath),
			Title:     assetsdb.Title(stem),
			ID:        assetsdb.ItemID(spec.ID, atlasPath, sub.Name),
			Source:    spec.ID,
			Kind:      assetsdb.KindSprite2D,
			Tokens:    assetsdb.Tokenize(sub.Name),
			Region:    &assetsdb.Region{X: sub.X, Y: sub.Y, Width: sub.Width, Height: sub.Height},
		})
	}

	return items
}

// fileItem builds the Item for a non-atlas zip entry f, or ok=false if assetsdb.Classify skips
// its extension.
func fileItem(spec assetsdb.PackSpec, f *zip.File, names *assetsdb.NameAllocator) (assetsdb.Item, bool, error) {
	kind, ok := assetsdb.Classify(f.Name)
	if !ok {
		return assetsdb.Item{}, false, nil
	}

	rc, err := f.Open()
	if err != nil {
		return assetsdb.Item{}, false, fmt.Errorf("kenney: open %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	contents, err := io.ReadAll(rc)
	if err != nil {
		return assetsdb.Item{}, false, fmt.Errorf("kenney: read %s: %w", f.Name, err)
	}

	stem := strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name))

	item := assetsdb.Item{
		Name:      names.Allocate(assetsdb.Slugify(spec.ID + "-" + stem)),
		Path:      f.Name,
		Format:    assetsdb.ExtNoDot(f.Name),
		MediaType: assetsdb.MediaType(f.Name),
		Bytes:     int64(len(contents)),
		Hash:      assetsdb.Hash(contents),
		Title:     assetsdb.Title(stem),
		ID:        assetsdb.ItemID(spec.ID, f.Name, ""),
		Source:    spec.ID,
		Kind:      kind,
		Tokens:    assetsdb.Tokenize(f.Name),
	}

	return item, true, nil
}
