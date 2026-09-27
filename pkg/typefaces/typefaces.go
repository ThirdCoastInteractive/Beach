// Package typefaces adapts Rewind's Google Fonts utilities for Beach. Catalog
// discovery uses Fontsource; installed fonts are served locally and TTF paths
// can be passed to caption/title export tools. Nothing downloads during render.
package typefaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CatalogItem struct {
	ID       string `json:"id"`
	Family   string `json:"family"`
	Category string `json:"category"`
	Variable bool   `json:"variable"`
	Type     string `json:"type,omitempty"`
}
type File struct {
	Weight int    `json:"weight"`
	Format string `json:"format"`
	Name   string `json:"name"`
}
type Face struct {
	ID, Family, Category string
	Files                []File
}

// Library owns an explicitly configured directory. Independent applications do
// not share process-global font settings. The HTTP client is injectable in tests.
type Library struct {
	root   string
	client *http.Client
	mu     sync.Mutex
}

func New(dir string, client *http.Client) (*Library, error) {
	if dir == "" {
		return nil, errors.New("typefaces: directory required")
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many font redirects")
		}
		return trustedURL(req.URL.String())
	}
	return &Library{root: root, client: &copyClient}, nil
}

const catalogURL = "https://api.fontsource.org/v1/fonts"

var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var filePattern = regexp.MustCompile(`^latin-[1-9]00-normal\.(ttf|woff2)$`)

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
func trustedURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return errors.New("typefaces: invalid font URL")
	}
	switch u.Hostname() {
	case "api.fontsource.org", "cdn.jsdelivr.net", "cdn.fontsource.org", "fonts.gstatic.com":
		return nil
	}
	return fmt.Errorf("typefaces: unexpected font host %q", u.Hostname())
}
func (l *Library) fetch(ctx context.Context, raw string, limit int64) ([]byte, error) {
	if err := trustedURL(raw); err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("User-Agent", "Beach/typefaces")
	res, err := l.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("typefaces: HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("typefaces: response too large")
	}
	return b, nil
}
func (l *Library) catalog(ctx context.Context) ([]CatalogItem, error) {
	b, err := l.fetch(ctx, catalogURL, 8<<20)
	if err != nil {
		return nil, err
	}
	var list []CatalogItem
	err = json.Unmarshal(b, &list)
	return list, err
}

// Search returns featured families offline for an empty query, or searches the
// Google corpus by family/id and category. Results are capped at 80 families.
func (l *Library) Search(ctx context.Context, q, category string, limit int) ([]CatalogItem, error) {
	if limit <= 0 || limit > 80 {
		limit = 40
	}
	q = strings.ToLower(strings.TrimSpace(q))
	category = NormalizeCategory(category)
	list := Popular
	if q == "" && category == "blackletter" {
		list = Blackletter
	}
	if q != "" {
		var err error
		list, err = l.catalog(ctx)
		if err != nil {
			return nil, err
		}
	}
	out := []CatalogItem{}
	for _, f := range list {
		if f.Type != "" && f.Type != "google" {
			continue
		}
		if (q == "" || strings.Contains(strings.ToLower(f.Family), q) || strings.Contains(f.ID, q)) && itemMatchesCat(f, category) {
			out = append(out, f)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

type detail struct {
	Variants map[string]map[string]map[string]struct {
		URL map[string]string `json:"url"`
	} `json:"variants"`
}

// Install downloads latin normal 400/700 web and export faces where available.
// A family appears in Installed only after every selected file is complete.
// Existing installations are reused; failure leaves them intact.
func (l *Library) Install(ctx context.Context, family string) (Face, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if f, err := l.read(slug(family)); err == nil {
		return f, nil
	}
	list, err := l.catalog(ctx)
	if err != nil {
		return Face{}, err
	}
	var item CatalogItem
	for _, f := range list {
		if (f.Type == "" || f.Type == "google") && (f.ID == slug(family) || strings.EqualFold(f.Family, family)) {
			item = f
			break
		}
	}
	if !idPattern.MatchString(item.ID) {
		return Face{}, fmt.Errorf("typefaces: unknown family %q", family)
	}
	if f, err := l.read(item.ID); err == nil {
		return f, nil
	}
	b, err := l.fetch(ctx, catalogURL+"/"+item.ID, 4<<20)
	if err != nil {
		return Face{}, err
	}
	var d detail
	if err = json.Unmarshal(b, &d); err != nil {
		return Face{}, err
	}
	if err = os.MkdirAll(l.root, 0755); err != nil {
		return Face{}, err
	}
	tmp, err := os.MkdirTemp(l.root, ".install-")
	if err != nil {
		return Face{}, err
	}
	defer os.RemoveAll(tmp)
	f := Face{ID: item.ID, Family: item.Family, Category: item.Category}
	for _, weight := range []int{400, 700} {
		for _, format := range []string{"woff2", "ttf"} {
			raw := d.Variants[strconv.Itoa(weight)]["normal"]["latin"].URL[format]
			if raw == "" {
				continue
			}
			b, err = l.fetch(ctx, raw, 8<<20)
			if err != nil {
				return Face{}, err
			}
			valid := len(b) >= 12 && ((format == "woff2" && string(b[:4]) == "wOF2") || (format == "ttf" && (string(b[:4]) == "\x00\x01\x00\x00" || string(b[:4]) == "OTTO")))
			if !valid {
				return Face{}, errors.New("typefaces: invalid font file")
			}
			name := fmt.Sprintf("latin-%d-normal.%s", weight, format)
			if err = os.WriteFile(filepath.Join(tmp, name), b, 0644); err != nil {
				return Face{}, err
			}
			f.Files = append(f.Files, File{weight, format, name})
		}
	}
	if len(f.Files) == 0 {
		return Face{}, errors.New("typefaces: no supported latin faces")
	}
	b, _ = json.MarshalIndent(f, "", "  ")
	if err = os.WriteFile(filepath.Join(tmp, "family.json"), b, 0644); err != nil {
		return Face{}, err
	}
	if err = os.Rename(tmp, filepath.Join(l.root, item.ID)); err != nil {
		return Face{}, err
	}
	return f, nil
}
func (l *Library) read(id string) (Face, error) {
	if !idPattern.MatchString(id) {
		return Face{}, errors.New("typefaces: invalid family ID")
	}
	b, err := os.ReadFile(filepath.Join(l.root, id, "family.json"))
	if err != nil {
		return Face{}, err
	}
	var f Face
	if err = json.Unmarshal(b, &f); err != nil {
		return Face{}, err
	}
	if f.ID != id || len(f.Files) == 0 {
		return Face{}, errors.New("typefaces: invalid family metadata")
	}
	for _, file := range f.Files {
		if !filePattern.MatchString(file.Name) || file.Weight < 100 || file.Weight > 900 || (file.Format != "ttf" && file.Format != "woff2") {
			return Face{}, errors.New("typefaces: invalid file metadata")
		}
		if _, err := os.Stat(filepath.Join(l.root, id, file.Name)); err != nil {
			return Face{}, err
		}
	}
	return f, nil
}
func (l *Library) Installed() ([]Face, error) {
	ents, err := os.ReadDir(l.root)
	if os.IsNotExist(err) {
		return []Face{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Face{}
	for _, e := range ents {
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			continue
		}
		f, err := l.read(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Family < out[j].Family })
	return out, nil
}

// File resolves the nearest installed TTF weight for caption/export tooling.
func (l *Library) File(family string, weight int) (string, error) {
	faces, err := l.Installed()
	if err != nil {
		return "", err
	}
	for _, f := range faces {
		if f.ID != slug(family) && !strings.EqualFold(f.Family, family) {
			continue
		}
		best := ""
		distance := 10000
		for _, file := range f.Files {
			d := file.Weight - weight
			if d < 0 {
				d = -d
			}
			if file.Format == "ttf" && d < distance {
				best = filepath.Join(l.root, f.ID, file.Name)
				distance = d
			}
		}
		if best != "" {
			return best, nil
		}
	}
	return "", os.ErrNotExist
}

// Open exposes only installed font binaries for an application-owned file route.
// Metadata and staging directories are never exposed by this fs.FS.
func (l *Library) Open(name string) (fs.File, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 || !idPattern.MatchString(parts[0]) || !filePattern.MatchString(parts[1]) {
		return nil, os.ErrNotExist
	}
	f, err := l.read(parts[0])
	if err != nil {
		return nil, err
	}
	for _, file := range f.Files {
		if file.Name == parts[1] {
			return os.Open(filepath.Join(l.root, parts[0], parts[1]))
		}
	}
	return nil, os.ErrNotExist
}

// CSS returns same-origin @font-face declarations for installed families.
// The application serves this through its existing static/file infrastructure.
func (l *Library) CSS(prefix string) (string, error) {
	if !strings.HasPrefix(prefix, "/") || strings.HasPrefix(prefix, "//") || strings.ContainsAny(prefix, "\"'<>\\\r\n?#") {
		return "", errors.New("typefaces: local asset prefix required")
	}
	faces, err := l.Installed()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, f := range faces {
		for _, file := range f.Files {
			if file.Format != "woff2" {
				continue
			}
			fmt.Fprintf(&b, "@font-face{font-family:%s;src:url(%s) format(\"woff2\");font-weight:%d;font-style:normal;font-display:swap;}\n", cssString(f.Family), cssString(strings.TrimSuffix(prefix, "/")+"/"+f.ID+"/"+file.Name), file.Weight)
		}
	}
	return b.String(), nil
}
func cssString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r < ' ' || r == '"' || r == '\\' || r == '<' || r == '>' {
			fmt.Fprintf(&b, "\\%x ", r)
		} else {
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
