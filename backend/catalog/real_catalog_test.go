package catalog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Structural and provenance checks are offline: provider outages must not make CI
// flaky. Rechecking actual source contents remains an operator publication step.
func TestRealCatalogHasReviewedAdultPrograms(t *testing.T) {
	file, err := os.Open("../../data/real-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../research/catalog-review-2026-10-05.json")
	if err != nil {
		t.Fatal(err)
	}
	var audit struct {
		Courses []struct {
			ID            string    `json:"course_id"`
			Source        string    `json:"source"`
			Checked       time.Time `json:"checked_at"`
			Verification  string    `json:"verification"`
			Price         string    `json:"price_evidence"`
			Enrollment    string    `json:"enrollment_evidence"`
			Prerequisites string    `json:"prerequisites"`
			Support       string    `json:"support_evidence"`
		}
	}
	if err := json.Unmarshal(raw, &audit); err != nil {
		t.Fatal(err)
	}
	if len(data.Courses) < 20 || len(audit.Courses) != len(data.Courses) {
		t.Fatal("at least twenty individually reviewed programs required")
	}
	seen := map[string]bool{}
	languages := map[string]int{}
	free, paid, unknown, mentor := 0, 0, 0, 0
	for _, course := range data.Courses {
		if course.Demo || strings.HasPrefix(course.ID, "demo-") || course.Status != "published" || course.Summary == "" || len(course.Topics) == 0 {
			t.Fatalf("incomplete or demo course: %s", course.ID)
		}
		languages[course.Language]++
		matched := false
		for _, record := range audit.Courses {
			if record.ID == course.ID {
				if seen[record.ID] || record.Source != course.Source || !record.Checked.Equal(course.CheckedAt) || record.Verification != "official_public_page_read" || record.Price == "" || record.Enrollment == "" || record.Prerequisites == "" || record.Support == "" {
					t.Fatalf("missing or inconsistent provenance: %s", course.ID)
				}
				seen[record.ID] = true
				matched = true
			}
		}
		if !matched {
			t.Fatalf("no individual source review: %s", course.ID)
		}
		for _, offer := range course.Offers {
			if offer.Free {
				free++
			} else if offer.Price != nil {
				paid++
			} else {
				unknown++
			}
			if offer.Mentor {
				mentor++
			}
			if !offer.PriceCheckedAt.Equal(course.CheckedAt) {
				t.Fatal("price date differs from actual review")
			}
		}
	}
	for _, language := range []string{"go", "python", "java", "javascript"} {
		if languages[language] < 5 {
			t.Fatalf("less than five %s programs", language)
		}
	}
	if free == 0 || paid == 0 || unknown == 0 || mentor == 0 {
		t.Fatal("real catalog must represent free/confirmed paid/unknown prices and individual support")
	}
	for _, domain := range data.Domains {
		if domain == "example.com" {
			t.Fatal("placeholder domain in real catalog")
		}
	}
}
