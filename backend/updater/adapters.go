package updater

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// An observation contains only the fields actually verified in this fetch.
// Curated classification/description, other tariffs and unpublished states are
// never overwritten from scraped marketing text.
type Observation struct {
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	PriceUnknown bool       `json:"price_unknown,omitempty"`
	Price        *int64     `json:"price,omitempty"`
	Enrollment   string     `json:"enrollment,omitempty"`
	Schedule     string     `json:"schedule,omitempty"`
}

func decodeJSON(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(v); err != nil {
		return ErrSource
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrSource
	}
	return nil
}

type stepikCourse struct {
	Summary         string          `json:"summary"`
	Lessons         int             `json:"lessons_count"`
	Units           int             `json:"total_units"`
	ContentLanguage string          `json:"language"`
	ID              int             `json:"id"`
	Title           string          `json:"title"`
	Public          *bool           `json:"is_public"`
	Active          *bool           `json:"is_active"`
	Enabled         *bool           `json:"is_enabled"`
	Archived        *bool           `json:"is_archived"`
	Censored        *bool           `json:"is_censored"`
	Paid            *bool           `json:"is_paid"`
	SelfPaced       *bool           `json:"is_self_paced"`
	Price           json.RawMessage `json:"price"`
	Currency        string          `json:"currency_code"`
	EnrollmentBegin *time.Time      `json:"enrollment_begin_date"`
	EnrollmentEnd   *time.Time      `json:"enrollment_end_date"`
	Actions         map[string]struct {
		Enabled *bool `json:"enabled"`
	} `json:"actions"`
}

func normalizeTitle(s string) string {
	r := strings.NewReplacer("«", "\"", "»", "\"", "“", "\"", "”", "\"", "ё", "е")
	return strings.Join(strings.Fields(strings.ToLower(r.Replace(s))), " ")
}
func parseStepik(data []byte, s Source, now time.Time) (Observation, error) {
	var payload struct {
		Courses []stepikCourse `json:"courses"`
	}
	if decodeJSON(data, &payload) != nil || len(payload.Courses) != 1 {
		return Observation{}, ErrSource
	}
	c := payload.Courses[0]
	if c.ID != s.RemoteID || normalizeTitle(c.Title) != normalizeTitle(s.ExpectedTitle) || c.Public == nil || c.Active == nil || c.Enabled == nil || c.Archived == nil || c.Censored == nil || c.Paid == nil || !*c.Public || *c.Censored {
		return Observation{}, ErrSource
	}
	if *c.Archived {
		return Observation{Enrollment: "closed"}, nil
	}
	if !*c.Active || !*c.Enabled {
		return Observation{}, ErrSource
	}
	o := Observation{}
	if !*c.Paid {
		// Some free courses have price=null. An explicit is_paid=false is the
		// platform's price evidence; a contradictory nonzero price is rejected.
		if len(c.Price) > 0 && string(c.Price) != "null" {
			p, err := rubles(c.Price)
			if err != nil || p != 0 {
				return o, ErrSource
			}
		}
		zero := int64(0)
		o.Price = &zero
	} else if len(c.Price) > 0 && string(c.Price) != "null" && c.Currency == "RUB" {
		p, err := rubles(c.Price)
		if err != nil || p <= 0 {
			return o, ErrSource
		}
		o.Price = &p
	} else {
		o.PriceUnknown = true
	}
	if c.EnrollmentEnd != nil && !now.Before(*c.EnrollmentEnd) {
		o.Enrollment = "closed"
	} else if c.EnrollmentBegin != nil && now.Before(*c.EnrollmentBegin) {
		o.Enrollment = "closed"
	} else if available, ok := c.Actions["can_be_enrolled"]; ok && available.Enabled != nil && *available.Enabled {
		o.Enrollment = "open"
		if c.SelfPaced != nil && *c.SelfPaced && c.EnrollmentBegin == nil && c.EnrollmentEnd == nil {
			o.Enrollment = "continuous"
			o.Schedule = "flexible"
		}
	}
	// can_be_enrolled=false can mean account restrictions, not a closed course.
	if o.Price == nil && !o.PriceUnknown && o.Enrollment == "" {
		return o, ErrSource
	}
	return o, nil
}

var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,7})(\.[0-9]{1,2})?$`)

func rubles(raw []byte) (int64, error) {
	s := string(raw)
	if len(s) > 0 && s[0] == '"' {
		if json.Unmarshal(raw, &s) != nil {
			return 0, ErrSource
		}
	}
	if !decimal.MatchString(s) {
		return 0, ErrSource
	}
	parts := strings.Split(s, ".")
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	fraction := int64(0)
	if len(parts) == 2 {
		f := parts[1]
		if len(f) == 1 {
			f += "0"
		}
		fraction, _ = strconv.ParseInt(f, 10, 64)
	}
	return whole*100 + fraction, nil
}

// Structured HTML is accepted only with Course identity, explicit full-course
// free access, an exact zero RUB offer and explicit availability. A page's
// unrelated paid recommendations/intro lessons cannot satisfy this contract.
var jsonLD = regexp.MustCompile(`(?is)<script\b[^>]*\btype\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script\s*>`)

func parseSchemaCourse(data []byte, canonical string) (Observation, error) {
	var observations []Observation
	for _, m := range jsonLD.FindAllSubmatch(data, -1) {
		var document any
		if decodeJSON(m[1], &document) != nil {
			return Observation{}, ErrSource
		}
		walkSchema(document, canonical, &observations)
	}
	if len(observations) != 1 {
		return Observation{}, ErrSource
	}
	return observations[0], nil
}
func walkSchema(document any, canonical string, out *[]Observation) {
	switch v := document.(type) {
	case []any:
		for _, node := range v {
			walkSchema(node, canonical, out)
		}
	case map[string]any:
		if graph, ok := v["@graph"]; ok {
			walkSchema(graph, canonical, out)
		}
		if v["@type"] != "Course" || v["url"] != canonical || v["isAccessibleForFree"] != true {
			return
		}
		name, ok := v["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return
		}
		offers, ok := v["offers"].(map[string]any)
		if !ok || offers["@type"] != "Offer" || offers["priceCurrency"] != "RUB" {
			return
		}
		if _, has := offers["priceSpecification"]; has {
			return
		}
		if _, has := offers["eligibleDuration"]; has {
			return
		}
		raw, _ := json.Marshal(offers["price"])
		p, err := rubles(raw)
		if err != nil || p != 0 {
			return
		}
		availability, ok := offers["availability"].(string)
		if !ok {
			return
		}
		enrollment := ""
		switch availability {
		case "https://schema.org/InStock", "http://schema.org/InStock":
			enrollment = "continuous"
		case "https://schema.org/OutOfStock", "http://schema.org/OutOfStock":
			enrollment = "closed"
		default:
			return
		}
		*out = append(*out, Observation{Price: &p, Enrollment: enrollment, Schedule: "flexible"})
	}
}
func parse(data []byte, s Source, canonical string, now time.Time) (Observation, error) {
	switch s.Adapter {
	case "stepik":
		return parseStepik(data, s, now)
	case "schema-course":
		return parseSchemaCourse(data, canonical)
	}
	return Observation{}, errors.New("unsupported adapter")
}
func endpoint(s Source, canonical string) string {
	if s.Adapter == "stepik" {
		return fmt.Sprintf("https://stepik.org/api/courses/%d", s.RemoteID)
	}
	return canonical
}
