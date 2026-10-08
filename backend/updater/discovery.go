package updater

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Feeds contain official public sitemaps, never search-engine results. A URL in
// a feed is only a candidate: no price, course or availability is inferred here.
type Feed struct {
	ID      string `json:"id"`
	Adapter string `json:"adapter"`
	URL     string `json:"url"`
}
type Candidate struct {
	Adapter    string
	ExternalID string
	URL        string
	FeedID     string
}

var providerHosts = map[string]string{"stepik": "stepik.org", "otus": "otus.ru", "yandex": "practicum.yandex.ru", "hexlet": "ru.hexlet.io", "codebasics": "code-basics.com"}
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`)
var stepikPath = regexp.MustCompile(`^/course/(?:[a-z0-9-]+-)?([1-9][0-9]*)/promo$`)

func canonical(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Host == "" {
		return "", ErrSource
	}
	// Identity ignores tracking parameters and a cosmetic trailing slash. Pages
	// are fetched using the canonical URL, not the untrusted query/fragment.
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	if u.RawPath != "" {
		return "", ErrSource
	}
	return u.String(), nil
}
func candidateURL(adapter, raw, feedID string) (Candidate, bool) {
	clean, err := canonical(raw)
	if err != nil {
		return Candidate{}, false
	}
	u, _ := url.Parse(clean)
	if u.Host != providerHosts[adapter] {
		return Candidate{}, false
	}
	id := ""
	switch adapter {
	case "stepik":
		m := stepikPath.FindStringSubmatch(u.Path)
		if len(m) != 2 {
			return Candidate{}, false
		}
		id = m[1]
		if _, err := strconv.Atoi(id); err != nil {
			return Candidate{}, false
		}
		clean = "https://stepik.org/course/" + id + "/promo"
	case "otus":
		id = strings.TrimPrefix(u.Path, "/lessons/")
		if id == u.Path || !slugPattern.MatchString(id) {
			return Candidate{}, false
		}
	case "hexlet":
		id = strings.TrimPrefix(u.Path, "/programs/")
		if id == u.Path || !slugPattern.MatchString(id) {
			return Candidate{}, false
		}
	case "codebasics":
		id = strings.TrimPrefix(u.Path, "/ru/languages/")
		if id == u.Path || !slugPattern.MatchString(id) {
			return Candidate{}, false
		}
	case "yandex":
		id = strings.TrimPrefix(u.Path, "/")
		if !slugPattern.MatchString(id) {
			return Candidate{}, false
		}
	default:
		return Candidate{}, false
	}
	return Candidate{Adapter: adapter, ExternalID: id, URL: clean, FeedID: feedID}, true
}
func validateFeed(f Feed) error {
	if _, ok := providerHosts[f.Adapter]; !ok {
		return errors.New("unknown sitemap provider")
	}
	u, err := url.Parse(f.URL)
	if err != nil || !slugPattern.MatchString(f.ID) || u.Scheme != "https" || u.Host != providerHosts[f.Adapter] || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(u.Path, ".xml") {
		return errors.New("invalid official sitemap feed")
	}
	return nil
}
func sitemap(data []byte) (kind string, urls []string, err error) {
	if len(data) > maxBody {
		return "", nil, ErrSource
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		z, e := gzip.NewReader(bytes.NewReader(data))
		if e != nil {
			return "", nil, ErrSource
		}
		defer z.Close()
		data, e = io.ReadAll(io.LimitReader(z, maxBody+1))
		if e != nil || len(data) > maxBody {
			return "", nil, ErrSource
		}
	}
	var document struct {
		XMLName xml.Name
		URLs    []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
		Maps []struct {
			Loc string `xml:"loc"`
		} `xml:"sitemap"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&document) != nil {
		return "", nil, ErrSource
	}
	// Reject a second document or other trailing data, including error pages.
	for {
		t, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", nil, ErrSource
		}
		if c, ok := t.(xml.CharData); !ok || strings.TrimSpace(string(c)) != "" {
			return "", nil, ErrSource
		}
	}
	switch document.XMLName.Local {
	case "urlset":
		for _, entry := range document.URLs {
			urls = append(urls, entry.Loc)
		}
	case "sitemapindex":
		for _, entry := range document.Maps {
			urls = append(urls, entry.Loc)
		}
	default:
		return "", nil, ErrSource
	}
	if len(urls) > 100000 {
		return "", nil, ErrSource
	}
	return document.XMLName.Local, urls, nil
}
func childSitemaps(f Feed, urls []string) []string {
	var safe []string
	for _, raw := range urls {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.Host != providerHosts[f.Adapter] || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !(strings.HasSuffix(u.Path, ".xml") || strings.HasSuffix(u.Path, ".xml.gz")) {
			continue
		}
		// Stepik indexes include authors and individual exercises: only course
		// promo maps can seed course identities.
		if f.Adapter == "stepik" && !strings.Contains(u.Path, "sitemap-course-promo-") {
			continue
		}
		safe = append(safe, raw)
	}
	return safe
}
func (s *Service) discoverFeed(ctx context.Context, client *http.Client, f Feed) (int, error) {
	body, _, err := fetch(ctx, client, f.URL)
	if err != nil {
		return 0, err
	}
	kind, urls, err := sitemap(body)
	if err != nil {
		return 0, err
	}
	cursor := 0
	if kind == "sitemapindex" {
		children := childSitemaps(f, urls)
		if len(children) == 0 {
			return 0, ErrSource
		}
		if err = s.DB.Pool.QueryRow(ctx, `INSERT INTO discovery_feeds(feed_id) VALUES($1) ON CONFLICT(feed_id) DO UPDATE SET feed_id=EXCLUDED.feed_id RETURNING cursor`, f.ID).Scan(&cursor); err != nil {
			return 0, err
		}
		child := children[cursor%len(children)]
		// Advance even if one shard is unavailable, so a failed shard cannot starve
		// the remaining official course sitemaps.
		if _, err = s.DB.Pool.Exec(ctx, "UPDATE discovery_feeds SET cursor=$2 WHERE feed_id=$1", f.ID, (cursor+1)%len(children)); err != nil {
			return 0, err
		}
		body, _, err = fetch(ctx, client, child)
		if err != nil {
			return 0, err
		}
		kind, urls, err = sitemap(body)
		if err != nil || kind != "urlset" {
			return 0, ErrSource
		}
	}
	count := 0
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	for _, raw := range urls {
		candidate, ok := candidateURL(f.Adapter, raw, f.ID)
		if !ok {
			continue
		}
		// Official path hints limit large general-provider sitemaps; the detail
		// parser independently checks the actual language before publishing.
		if f.Adapter != "stepik" && languageHint(candidate.ExternalID) == "" {
			continue
		}
		tag, e := tx.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id)
   SELECT $1,$2,$3,$4 WHERE (SELECT count(*) FROM catalog_candidates)<50000
   ON CONFLICT(adapter,external_id) DO UPDATE SET last_seen_at=now(),feed_id=EXCLUDED.feed_id
   WHERE catalog_candidates.canonical_url=EXCLUDED.canonical_url`, candidate.Adapter, candidate.ExternalID, candidate.URL, candidate.FeedID)
		if e != nil {
			return 0, e
		}
		count += int(tag.RowsAffected())
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}
func fingerprint(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func languageHint(text string) string {
	text = strings.ToLower(text)
	for _, pair := range [][2]string{{"javascript", "javascript"}, {"python", "python"}, {"golang", "go"}, {"java", "java"}, {"frontend", "javascript"}, {"backend-developer", "python"}, {"go-", "go"}} {
		if strings.Contains(text, pair[0]) {
			return pair[1]
		}
	}
	if text == "go" {
		return "go"
	}
	return ""
}
