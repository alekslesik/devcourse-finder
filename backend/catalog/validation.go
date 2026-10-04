package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Dataset struct {
	Domains []string `json:"approved_domains"`
	Courses []Course `json:"courses"`
}

// ValidationError carries a fixed validator rule and an input position. The CLI
// logs these rather than Error(), which includes an untrusted course identifier.
type ValidationError struct {
	CourseID string
	Index    int
	Rule     string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("course %q: %s", e.CourseID, e.Rule) }

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`)

func SafeURL(raw string, domains []string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || u.Port() != "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if net.ParseIP(h) != nil || h == "localhost" || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return false
	}
	return slices.Contains(domains, h)
}
func Decode(r io.Reader) (Dataset, error) {
	var d Dataset
	dec := json.NewDecoder(io.LimitReader(r, 20<<20))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&d); e != nil {
		return d, e
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return d, fmt.Errorf("expected one JSON document")
	}
	ids := map[string]bool{}
	slugs := map[string]bool{}
	offers := map[string]bool{}
	for index, c := range d.Courses {
		fail := func(s string) error { return &ValidationError{CourseID: c.ID, Index: index, Rule: s} }
		if !idPattern.MatchString(c.ID) || ids[c.ID] || !idPattern.MatchString(c.Slug) || slugs[c.Slug] {
			return d, fail("invalid or duplicate id/slug")
		}
		ids[c.ID] = true
		slugs[c.Slug] = true
		if c.Title == "" || c.Provider == "" || !slices.Contains([]string{"go", "python", "java", "javascript"}, c.Language) || !slices.Contains([]string{"basics", "backend", "frontend", "fullstack", "automation"}, c.Direction) {
			return d, fail("required name, provider, language, direction")
		}
		if !slices.Contains([]string{"draft", "published", "archived"}, c.Status) || c.CheckedAt.IsZero() || c.CheckedAt.After(time.Now().Add(24*time.Hour)) || !SafeURL(c.Source, d.Domains) {
			return d, fail("invalid status, date or source")
		}
		if len(c.Audience) == 0 || len(c.Goals) == 0 || len(c.Offers) == 0 {
			return d, fail("audience, goals and offers required")
		}
		for _, v := range c.Audience {
			if !slices.Contains([]string{"none", "basics", "projects", "working", "switch"}, v) {
				return d, fail("invalid audience")
			}
		}
		for _, v := range c.Goals {
			if !slices.Contains([]string{"try", "job", "switch", "deepen"}, v) {
				return d, fail("invalid goal")
			}
		}
		for _, o := range c.Offers {
			if !idPattern.MatchString(o.ID) || offers[o.ID] {
				return d, fail("invalid or duplicate offer id")
			}
			offers[o.ID] = true
			if o.Name == "" || !SafeURL(o.URL, d.Domains) {
				return d, fail("offer name or URL invalid")
			}
			if !slices.Contains([]string{"exact", "from", "unknown"}, o.PriceKind) || !slices.Contains([]string{"open", "continuous", "closed", "unknown"}, o.Enrollment) || !slices.Contains([]string{"flexible", "scheduled", "unknown"}, o.Schedule) {
				return d, fail("invalid offer enum")
			}
			if o.Price != nil && (*o.Price < 0 || (*o.Price == 0 && !o.Free)) {
				return d, fail("invalid price")
			}
			if o.PriceKind == "unknown" && o.Price != nil || o.PriceKind != "unknown" && o.Price == nil {
				return d, fail("price and kind inconsistent")
			}
			if o.Free && (o.Price == nil || *o.Price != 0 || o.PriceKind != "exact") {
				return d, fail("free must have exact zero price")
			}
			if o.Price != nil && (o.PriceCheckedAt.IsZero() || o.PriceCheckedAt.After(time.Now().Add(24*time.Hour))) {
				return d, fail("price verification date required")
			}
			if o.Hours != nil && *o.Hours <= 0 || o.Weeks != nil && *o.Weeks <= 0 {
				return d, fail("hours/weeks must be positive")
			}
		}
	}
	return d, nil
}
