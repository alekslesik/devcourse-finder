// catalog-audit compares public search counts against backend rules over every
// visible course. It performs no database writes or model requests.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	"devcourse-finder/catalog"
)

type response struct {
	Items            []catalog.Result `json:"items"`
	Total            int              `json:"total"`
	Confirmed        int              `json:"confirmed_total"`
	Unverified       int              `json:"unverified_total"`
	UnknownAvailable int              `json:"unknown_available"`
	Page             int              `json:"page"`
	PageSize         int              `json:"page_size"`
}
type row struct {
	Language   string   `json:"language"`
	Facet      string   `json:"facet"`
	Selected   string   `json:"selected"`
	Eligible   int      `json:"eligible"`
	Confirmed  int      `json:"confirmed"`
	Unknown    int      `json:"unknown"`
	Excluded   int      `json:"excluded"`
	APITotal   int      `json:"api_total"`
	APIUnknown int      `json:"api_unknown_available"`
	Match      bool     `json:"matches_backend"`
	UnknownIDs []string `json:"unknown_ids"`
}
type report struct {
	Started   time.Time      `json:"started_at"`
	Finished  time.Time      `json:"finished_at"`
	Courses   int            `json:"snapshot_courses"`
	Snapshot  string         `json:"snapshot_digest"`
	Stable    bool           `json:"stable_snapshot"`
	Pass      bool           `json:"pass"`
	Providers map[string]int `json:"providers"`
	Rows      []row          `json:"rows"`
}
type auditor struct {
	client *http.Client
	base   string
}

func (a auditor) get(ctx context.Context, q url.Values) (response, error) {
	var out response
	q.Set("page_size", "48")
	req, err := http.NewRequestWithContext(ctx, "GET", a.base+"/api/v1/courses?"+q.Encode(), nil)
	if err != nil {
		return out, err
	}
	res, err := a.client.Do(req)
	if err != nil {
		return out, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return out, errors.New("catalog request failed")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil || len(raw) > 8<<20 {
		return out, errors.New("catalog response too large or unreadable")
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, errors.New("invalid catalog response")
	}
	if out.Total < 0 || out.Total > 10000 || out.PageSize != 48 || out.Page < 1 {
		return out, errors.New("invalid pagination")
	}
	return out, nil
}
func (a auditor) snapshot(ctx context.Context) ([]catalog.Course, string, error) {
	courses := []catalog.Course{}
	seen := map[string]bool{}
	total := -1
	for page := 1; ; page++ {
		res, err := a.get(ctx, url.Values{"page": {fmt.Sprint(page)}})
		if err != nil {
			return nil, "", err
		}
		if total < 0 {
			total = res.Total
		}
		if total != res.Total || res.Page != page {
			return nil, "", errors.New("catalog changed during pagination")
		}
		expected := min(48, total-len(courses))
		if len(res.Items) != expected {
			return nil, "", errors.New("incomplete catalog page")
		}
		for _, item := range res.Items {
			if item.Course.ID == "" || seen[item.Course.ID] {
				return nil, "", errors.New("duplicate or missing course identity")
			}
			seen[item.Course.ID] = true
			courses = append(courses, item.Course)
		}
		if len(courses) == total {
			break
		}
	}
	sort.Slice(courses, func(i, j int) bool { return courses[i].ID < courses[j].ID })
	raw, _ := json.Marshal(courses)
	return courses, fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}
func (a auditor) run(ctx context.Context) (out report, err error) {
	out.Started = time.Now().UTC()
	out.Pass = true
	out.Providers = map[string]int{}
	defer func() {
		out.Finished = time.Now().UTC()
		if err != nil {
			out.Pass = false
		}
	}()
	courses, digest, err := a.snapshot(ctx)
	if err != nil {
		return out, err
	}
	out.Courses = len(courses)
	out.Snapshot = digest
	for _, c := range courses {
		u, _ := url.Parse(c.Source)
		out.Providers[u.Hostname()]++
	}
	for _, language := range []string{"python", "javascript", "java", "go"} {
		base, _ := catalog.Parse(url.Values{"language": {language}})
		eligible := catalog.Search(courses, base, out.Started)
		for _, facet := range []struct {
			key    string
			values []string
		}{
			{"experience", []string{"none", "basics", "projects", "working", "switch", "experienced"}},
			{"goal", []string{"try", "job", "switch", "deepen"}},
		} {
			for _, selected := range facet.values {
				q := url.Values{"language": {language}, facet.key: {selected}, "diagnostics": {"true"}}
				f, _ := catalog.Parse(q)
				r := row{Language: language, Facet: facet.key, Selected: selected, Eligible: len(eligible), UnknownIDs: []string{}}
				for _, item := range eligible {
					switch catalog.MatchProfile(item.Course, facet.key, selected) {
					case "confirmed":
						r.Confirmed++
					case "unknown":
						r.Unknown++
						r.UnknownIDs = append(r.UnknownIDs, item.Course.ID)
					default:
						r.Excluded++
					}
				}
				api, e := a.get(ctx, q)
				if e != nil {
					return out, e
				}
				r.APITotal = api.Total
				r.APIUnknown = api.UnknownAvailable
				expected := catalog.Search(courses, f, out.Started)
				r.Match = len(expected) == r.Confirmed && api.Total == len(expected) && api.Confirmed == r.Confirmed && api.Unverified == 0 && api.UnknownAvailable == r.Unknown
				// The opt-in path must also expose precisely those unknown profiles.
				q.Set("include_unknown_profiles", "true")
				expanded, e := a.get(ctx, q)
				if e != nil {
					return out, e
				}
				r.Match = r.Match && expanded.Total == r.Confirmed+r.Unknown && expanded.Confirmed == r.Confirmed && expanded.Unverified == r.Unknown
				sort.Strings(r.UnknownIDs)
				out.Rows = append(out.Rows, r)
				out.Pass = out.Pass && r.Match
			}
		}
	}
	_, second, err := a.snapshot(ctx)
	if err != nil {
		return out, err
	}
	out.Stable = second == digest
	out.Pass = out.Pass && out.Stable
	if !out.Stable {
		return out, errors.New("catalog changed during audit; retry")
	}
	return out, nil
}
func main() {
	base := flag.String("url", "", "public site origin, for example https://example.com")
	flag.Parse()
	u, err := url.Parse(*base)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		fmt.Fprintln(os.Stderr, "Supply a public site origin with --url.")
		os.Exit(2)
	}
	u.Path = ""
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	a := auditor{base: u.String(), client: &http.Client{Timeout: 15 * time.Second}}
	result, err := a.run(ctx)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	if err != nil || !result.Pass {
		os.Exit(1)
	}
}
