package updater

import (
	"bytes"
	"io"
	"strconv"
	"time"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

// CodeBasics publishes its built, complete curriculum in the anonymous Inertia
// response. Its current page has no Offer/free flag: require a fresh official
// platform pricing statement as well, never infer free from a missing price.
func codeBasicsFreePolicy(data []byte) bool {
	for _, script := range jsonLD.FindAllSubmatch(data, -1) {
		var faq struct {
			Type      string `json:"@type"`
			Questions []struct {
				Type   string `json:"@type"`
				Name   string `json:"name"`
				Answer struct {
					Type string `json:"@type"`
					Text string `json:"text"`
				} `json:"acceptedAnswer"`
			} `json:"mainEntity"`
		}
		if decodeJSON(script[1], &faq) != nil || faq.Type != "FAQPage" {
			continue
		}
		for _, q := range faq.Questions {
			text := plainText(q.Answer.Text, 2000)
			// This is the recorded platform-wide pricing contract, not a price
			// phrase taken from an exercise, advertisement or introductory lesson.
			if q.Type == "Question" && q.Name == "Сколько стоят курсы на платформе?" && q.Answer.Type == "Answer" && text == "Code Basics создавался как проект для обучения программированию с нуля бесплатно. Таким он, был, есть и будет. Более того, Code Basics это открытый проект, код которого можно не только найти на Github, но и принять участие в его разработке" {
				return true
			}
		}
	}
	return false
}

type basicsLesson struct {
	Slug     string `json:"slug"`
	Language struct {
		Slug string `json:"slug"`
	} `json:"language"`
}
type basicsPage struct {
	Component string `json:"component"`
	URL       string `json:"url"`
	Props     struct {
		Locale string `json:"locale"`
		Course struct {
			ID      int    `json:"id"`
			Slug    string `json:"slug"`
			Version struct {
				Result string `json:"result"`
				State  string `json:"state"`
			} `json:"current_version"`
		} `json:"course"`
		Landing struct {
			LanguageID  int    `json:"language_id"`
			Slug        string `json:"slug"`
			Main        bool   `json:"main"`
			Listed      bool   `json:"listed"`
			State       string `json:"state"`
			Header      string `json:"header"`
			Description string `json:"description"`
		} `json:"courseLandingPage"`
		First   basicsLesson `json:"firstLesson"`
		Modules []struct {
			ID     int    `json:"id"`
			Locale string `json:"locale"`
		} `json:"courseModules"`
		Lessons map[string][]basicsLesson `json:"lessonsByModuleId"`
	} `json:"props"`
}

func collectCodeBasics(data, policy []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	if !codeBasicsFreePolicy(policy) || len(data) > maxBody || c.Adapter != "codebasics" {
		return catalog.Course{}, Observation{}, ErrSource
	}
	identity, ok := candidateURL(c.Adapter, c.URL, c.FeedID)
	if !ok || identity.ExternalID != c.ExternalID {
		return catalog.Course{}, Observation{}, ErrSource
	}
	tokenizer := html.NewTokenizer(bytes.NewReader(data))
	var page basicsPage
	pages := 0
	canonicalOK := false
	var lessonLinks = map[string]bool{}
	capture := false
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			if tokenizer.Err() != io.EOF {
				return catalog.Course{}, Observation{}, ErrSource
			}
			break
		}
		token := tokenizer.Token()
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			attrs := map[string]string{}
			for _, a := range token.Attr {
				attrs[a.Key] = a.Val
			}
			if token.Data == "link" && attrs["rel"] == "canonical" && matchingURL(attrs["href"], c.URL) {
				canonicalOK = true
			}
			if token.Data == "a" {
				lessonLinks[attrs["href"]] = true
			}
			if token.Data == "script" && attrs["data-page"] == "app" && attrs["type"] == "application/json" {
				capture = true
				pages++
			}
		} else if kind == html.TextToken && capture {
			capture = false
			if decodeJSON([]byte(token.Data), &page) != nil {
				return catalog.Course{}, Observation{}, ErrSource
			}
		}
	}
	p := page.Props

	identityOK := pages == 1 && canonicalOK && page.Component == "web/languages/show" &&
		page.URL == "/ru/languages/"+c.ExternalID && p.Locale == "ru" &&
		p.Course.Slug == c.ExternalID && p.Course.ID > 0 &&
		p.Landing.LanguageID == p.Course.ID && p.Landing.Slug == c.ExternalID
	published := p.Landing.Main && p.Landing.Listed && p.Landing.State == "published"
	ready := p.Course.Version.Result == "Success" && p.Course.Version.State == "built"
	startAvailable := p.First.Language.Slug == c.ExternalID && slugPattern.MatchString(p.First.Slug) &&
		lessonLinks[page.URL+"/lessons/"+p.First.Slug]
	if !identityOK || !published || !ready || !startAvailable {
		return catalog.Course{}, Observation{}, ErrSource
	}

	lessons := map[string]bool{}
	for _, module := range p.Modules {
		if module.ID < 1 || module.Locale != "ru" {
			return catalog.Course{}, Observation{}, ErrSource
		}
		for _, lesson := range p.Lessons[strconv.Itoa(module.ID)] {
			if lesson.Language.Slug != c.ExternalID || !slugPattern.MatchString(lesson.Slug) {
				return catalog.Course{}, Observation{}, ErrSource
			}
			lessons[lesson.Slug] = true
		}
	}
	if len(lessons) < 3 {
		return catalog.Course{}, Observation{}, ErrSource
	}
	zero := int64(0)
	o := Observation{Price: &zero, Enrollment: "continuous", Schedule: "flexible"}
	record, err := normalizedRecord(c, p.Landing.Header, p.Landing.Description, "CodeBasics", o, now)
	return record, o, err
}
