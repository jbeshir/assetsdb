// Package kaykit implements format.Ingester for KayKit-Game-Assets GitHub repositories: it
// downloads a pack's codeload ZIP archive, walks it to catalog 3D model and texture assets while
// deduplicating redundant model formats, and reports every pack's license as CC0-1.0 (KayKit's
// LICENSE.txt is manually confirmed CC0 even though GitHub's API reports NOASSERTION for it).
package kaykit

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// defaultOwner is the GitHub account KayKit packs are published under when a PackSpec.URL names
// only a repository, with no owner.
const defaultOwner = "KayKit-Game-Assets"

// userAgent identifies this ingester to the GitHub API and codeload, which both require a
// non-empty User-Agent header.
const userAgent = "assetsdb-kaykit-ingester/1.0 (+https://github.com/jbeshir/assetsdb)"

// ErrInvalidSpecURL is returned when a PackSpec's URL cannot be parsed into an owner/repo pair.
var ErrInvalidSpecURL = errors.New("kaykit: invalid spec URL")

// ErrDownloadFailed is returned when codeload responds with a non-200 status for an archive
// download.
var ErrDownloadFailed = errors.New("kaykit: archive download failed")

// cc0License is the license entry recorded for every KayKit pack; see the package doc for why
// this is hard-coded rather than trusted from the GitHub API.
var cc0License = format.License{
	Name:  "CC0-1.0",
	Path:  "https://creativecommons.org/publicdomain/zero/1.0/legalcode",
	Title: "Creative Commons Zero v1.0 Universal",
}

// Ingester implements format.Ingester for KayKit-Game-Assets GitHub repositories.
type Ingester struct {
	// HTTPClient performs all network requests; http.DefaultClient is used when nil.
	HTTPClient *http.Client
}

// Name returns this ingester's identifier, "kaykit".
func (i *Ingester) Name() string {
	return "kaykit"
}

// Ingest fetches spec's KayKit GitHub repository archive and returns its Source metadata, its
// Items, and the raw ZIP bytes to persist verbatim under sources/<id>.zip.
func (i *Ingester) Ingest(ctx context.Context, spec format.PackSpec) (format.Source, []format.Item, []byte, error) {
	owner, repo, err := parseOwnerRepo(spec.URL)
	if err != nil {
		return format.Source{}, nil, nil, err
	}

	branch := i.fetchDefaultBranch(ctx, owner, repo)

	data, err := i.downloadZip(ctx, owner, repo, branch)
	if err != nil {
		return format.Source{}, nil, nil, fmt.Errorf("kaykit: download %s/%s: %w", owner, repo, err)
	}

	items, err := walkZip(spec, data)
	if err != nil {
		return format.Source{}, nil, nil, fmt.Errorf("kaykit: walk %s/%s: %w", owner, repo, err)
	}

	src := format.Source{
		Name:     spec.ID,
		Title:    spec.Name,
		Path:     "sources/" + spec.ID + ".zip",
		Licenses: []format.License{cc0License},
		Type:     "curated",
		Origin:   spec.URL,
		Tags:     spec.Tags,
	}

	return src, items, data, nil
}

// client returns the HTTP client to use for requests, falling back to http.DefaultClient.
func (i *Ingester) client() *http.Client {
	if i.HTTPClient != nil {
		return i.HTTPClient
	}
	return http.DefaultClient
}

// repoInfo is the subset of the GitHub repository API response this ingester reads.
type repoInfo struct {
	DefaultBranch string `json:"default_branch"`
}

// fetchDefaultBranch reads owner/repo's default branch from the GitHub API. It falls back to
// "main" on any failure (network error, non-200 status, or a malformed/empty response), since a
// branch is always needed to build the codeload URL and "main" is KayKit's convention today.
func (i *Ingester) fetchDefaultBranch(ctx context.Context, owner, repo string) string {
	const fallback = "main"

	reqURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fallback
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := i.client().Do(req)
	if err != nil {
		return fallback
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fallback
	}

	var info repoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.DefaultBranch == "" {
		return fallback
	}

	return info.DefaultBranch
}

// downloadZip streams owner/repo's branch archive from codeload into memory. The response
// carries no Content-Length (chunked transfer), so the whole body is read rather than
// pre-allocated.
func (i *Ingester) downloadZip(ctx context.Context, owner, repo, branch string) ([]byte, error) {
	reqURL := fmt.Sprintf("https://codeload.github.com/%s/%s/zip/refs/heads/%s", owner, repo, branch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := i.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrDownloadFailed, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return data, nil
}

// ownerRepoURL matches a GitHub repository URL, capturing the owner and repo path segments.
var ownerRepoURL = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/?$`)

// parseOwnerRepo derives a GitHub owner/repo pair from a PackSpec.URL. It accepts a full GitHub
// URL ("https://github.com/<owner>/<repo>"), an "<owner>/<repo>" string, or a bare repo name (in
// which case owner defaults to defaultOwner).
func parseOwnerRepo(specURL string) (owner, repo string, err error) {
	s := strings.TrimSuffix(strings.TrimSpace(specURL), ".git")
	s = strings.TrimSuffix(s, "/")

	if m := ownerRepoURL.FindStringSubmatch(s); m != nil {
		return m[1], m[2], nil
	}

	switch parts := strings.Split(s, "/"); len(parts) {
	case 1:
		if parts[0] == "" {
			return "", "", fmt.Errorf("%w: %q", ErrInvalidSpecURL, specURL)
		}
		return defaultOwner, parts[0], nil
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("%w: %q", ErrInvalidSpecURL, specURL)
		}
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("%w: %q", ErrInvalidSpecURL, specURL)
	}
}

// modelFormatPriority ranks the redundant model formats KayKit ships per asset, lowest first.
// Only extensions present here participate in the "one item per asset" dedup in dedupModelFormats;
// other model3d formats (e.g. .stl, .dae) are always kept as-is.
var modelFormatPriority = map[string]int{
	".glb":  0,
	".gltf": 1,
	".fbx":  2,
	".obj":  3,
}

// candidate is a zip entry that survived classification and skip filtering, pending the
// model-format dedup pass.
type candidate struct {
	file *zip.File
	path string
	kind format.Kind
}

// walkZip is the pure, network-free core of this ingester: it classifies and deduplicates every
// file in an already-downloaded KayKit codeload archive into a sorted, deterministic Item slice.
// spec.ID namespaces each item's canonical id and Frictionless name.
func walkZip(spec format.PackSpec, data []byte) ([]format.Item, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("kaykit: open zip: %w", err)
	}

	candidates := dedupModelFormats(collectCandidates(zr.File))
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })

	items := make([]format.Item, 0, len(candidates))
	names := make(map[string]int, len(candidates))
	for _, c := range candidates {
		item, ierr := buildItem(spec, c, names)
		if ierr != nil {
			return nil, fmt.Errorf("kaykit: build item %s: %w", c.path, ierr)
		}
		items = append(items, item)
	}

	return items, nil
}

// collectCandidates classifies every regular file in files, dropping directories, files Classify
// doesn't recognize, and KayKit's known preview/marketing images.
func collectCandidates(files []*zip.File) []candidate {
	out := make([]candidate, 0, len(files))
	for _, f := range files {
		if f.FileInfo().IsDir() || isPreviewImage(f.Name) {
			continue
		}
		kind, ok := format.Classify(f.Name)
		if !ok {
			continue
		}
		out = append(out, candidate{file: f, path: f.Name, kind: kind})
	}
	return out
}

// isPreviewImage reports whether p is one of KayKit's non-asset preview/marketing images: the
// pack thumbnail, the addon contents sheet, or a rendered Samples/ preview.
func isPreviewImage(p string) bool {
	switch strings.ToLower(path.Base(p)) {
	case "icon.png", "contents.png":
		return true
	}
	for _, seg := range strings.Split(p, "/") {
		if strings.EqualFold(seg, "samples") {
			return true
		}
	}
	return false
}

// dedupModelFormats keeps only the highest-priority model file (see modelFormatPriority) for
// each distinct dir+basename asset, so a model shipped as both .fbx and .gltf yields one item.
// Candidates that aren't model3d, or are a model3d format outside modelFormatPriority, pass
// through unchanged.
func dedupModelFormats(candidates []candidate) []candidate {
	best := make(map[string]candidate)
	for _, c := range candidates {
		pri, participates := candidateModelPriority(c)
		if !participates {
			continue
		}
		key := assetKey(c.path)
		if cur, ok := best[key]; !ok || pri < mustCandidateModelPriority(cur) {
			best[key] = c
		}
	}

	keep := make(map[string]bool, len(best))
	for _, c := range best {
		keep[c.path] = true
	}

	out := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		if _, participates := candidateModelPriority(c); participates && !keep[c.path] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// candidateModelPriority returns c's model-format priority and whether it participates in
// per-asset dedup at all.
func candidateModelPriority(c candidate) (priority int, participates bool) {
	if c.kind != format.KindModel3D {
		return 0, false
	}
	pri, ok := modelFormatPriority[strings.ToLower(path.Ext(c.path))]
	return pri, ok
}

// mustCandidateModelPriority returns c's model-format priority; callers only invoke it for
// candidates already known to participate in dedup.
func mustCandidateModelPriority(c candidate) int {
	pri, _ := candidateModelPriority(c)
	return pri
}

// assetKey identifies the logical asset a model file belongs to, independent of its format
// extension: its directory plus its extension-stripped basename.
func assetKey(p string) string {
	return path.Dir(p) + "/" + basenameNoExt(p)
}

// buildItem reads c's contents from the still-open zip and constructs its catalog Item. names
// tracks Frictionless-name slugs already assigned within this walk, so a collision (two assets
// sharing a basename in different directories) gets a "-2", "-3", ... suffix.
func buildItem(spec format.PackSpec, c candidate, names map[string]int) (format.Item, error) {
	rc, err := c.file.Open()
	if err != nil {
		return format.Item{}, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = rc.Close() }()

	contents, err := io.ReadAll(rc)
	if err != nil {
		return format.Item{}, fmt.Errorf("read: %w", err)
	}

	sum := sha256.Sum256(contents)
	base := basenameNoExt(c.path)
	ext := strings.ToLower(path.Ext(c.path))

	return format.Item{
		Name:      uniqueSlug(spec.ID+"-"+slugify(base), names),
		Path:      c.path,
		Format:    strings.TrimPrefix(ext, "."),
		MediaType: format.MediaType(c.path),
		Bytes:     int64(len(contents)),
		Hash:      "sha256:" + hex.EncodeToString(sum[:]),
		Title:     humanizeTitle(base),
		ID:        fmt.Sprintf("assetsdb:%s/%s", spec.ID, c.path),
		Source:    spec.ID,
		Kind:      c.kind,
		Tokens:    format.Tokenize(c.path),
	}, nil
}

// basenameNoExt returns p's final path segment with its extension stripped.
func basenameNoExt(p string) string {
	b := path.Base(p)
	return strings.TrimSuffix(b, path.Ext(b))
}

// nonSlugChars matches any run of characters not permitted in a Frictionless resource name.
var nonSlugChars = regexp.MustCompile(`[^a-z0-9._-]+`)

// slugify lowercases s and replaces every run of non-slug characters with a single hyphen, so
// the result matches the Frictionless name pattern ^[a-z0-9._-]+$.
func slugify(s string) string {
	return nonSlugChars.ReplaceAllString(strings.ToLower(s), "-")
}

// uniqueSlug returns base the first time it's requested, then base suffixed with "-2", "-3", ...
// on each subsequent collision, recording the count in names.
func uniqueSlug(base string, names map[string]int) string {
	n := names[base]
	names[base] = n + 1
	if n == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, n+1)
}

// delimiterSplitter splits a filename stem on the explicit delimiters humanizeTitle treats as
// word boundaries.
var delimiterSplitter = regexp.MustCompile(`[_\-\s.]+`)

// wordSplitter finds camelCase words, acronym runs, and digit runs within a delimiter-free
// segment, matching format.Tokenize's own word-boundary rule.
var wordSplitter = regexp.MustCompile(`[A-Z]+[a-z]*|[a-z]+|[0-9]+`)

// humanizeTitle turns an extension-stripped basename like "chair_A" into a human-readable title
// like "Chair A": split on delimiters and camelCase/digit boundaries, then capitalize each word.
func humanizeTitle(base string) string {
	var words []string
	for _, part := range delimiterSplitter.Split(base, -1) {
		for _, w := range wordSplitter.FindAllString(part, -1) {
			words = append(words, capitalizeWord(w))
		}
	}
	if len(words) == 0 {
		return base
	}
	return strings.Join(words, " ")
}

// capitalizeWord upper-cases w's first rune, leaving the rest as-is (wordSplitter's acronym
// branch already yields runs like "HTTP" that should stay upper-case).
func capitalizeWord(w string) string {
	if w == "" {
		return w
	}
	r := []rune(w)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

// ensure Ingester satisfies format.Ingester at compile time.
var _ format.Ingester = (*Ingester)(nil)
