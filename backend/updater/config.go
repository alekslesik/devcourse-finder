// Package updater collects explicitly configured official sources and publishes
// only verified observations. Network collection never holds a publication lock.
package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"devcourse-finder/catalog"
)

type Source struct {
	CourseID      string `json:"course_id"`
	Adapter       string `json:"adapter"`
	RemoteID      int    `json:"remote_id,omitempty"`
	ExpectedTitle string `json:"expected_title,omitempty"`
}
type Config struct {
	Sources   []Source        `json:"sources"`
	Templates catalog.Dataset `json:"-"`
}

func LoadConfig(configFile, catalogFile string) (Config, error) {
	var c Config
	f, err := os.Open(configFile)
	if err != nil {
		return c, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&c); err != nil {
		return c, err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return c, errors.New("extra configuration data")
	}
	seed, err := os.Open(catalogFile)
	if err != nil {
		return c, err
	}
	defer seed.Close()
	c.Templates, err = catalog.Decode(seed)
	if err != nil {
		return c, err
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if len(c.Sources) == 0 || len(c.Sources) > 100 {
		return errors.New("expected 1 to 100 sources")
	}
	courses := map[string]catalog.Course{}
	for _, v := range c.Templates.Courses {
		courses[v.ID] = v
	}
	seen := map[string]bool{}
	for _, s := range c.Sources {
		course, ok := courses[s.CourseID]
		if !ok || seen[s.CourseID] || course.Demo || course.Status != "published" || len(course.Offers) != 1 {
			return fmt.Errorf("invalid template for %s", s.CourseID)
		}
		seen[s.CourseID] = true
		u, err := url.Parse(course.Source)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("invalid source URL")
		}
		if course.Offers[0].URL != course.Source {
			return errors.New("source and offer must identify the same page")
		}
		switch s.Adapter {
		case "stepik":
			if u.Host != "stepik.org" || u.Path != "/course/"+strconv.Itoa(s.RemoteID)+"/promo" || s.RemoteID < 1 || strings.TrimSpace(s.ExpectedTitle) == "" {
				return errors.New("invalid Stepik identity")
			}
		case "schema-course":
			if u.Host != "code-basics.com" || u.Path != "/ru/languages/"+course.Language {
				return errors.New("unsupported structured source")
			}
		default:
			return errors.New("unsupported source adapter")
		}
	}
	return nil
}
func (c Config) Template(id string) catalog.Course {
	for _, v := range c.Templates.Courses {
		if v.ID == id {
			return v
		}
	}
	panic("validated template not found")
}
