package updater

import (
	"os"
	"testing"
	"time"
)

func TestHexletLearningRequiresExactNamedTier(t *testing.T) {
	data, e := os.ReadFile("testdata/learning/hexlet.html")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "hexlet", URL: "https://ru.hexlet.io/programs/python"}
	now := time.Now().UTC()
	node := map[string]any{"offers": map[string]any{"@type": "Offer", "name": "Стандартный"}}
	v := hexletLearning(data, node, c, now)
	if v.Duration == nil || v.Duration.Max != 10 || v.Mentor != nil || v.WeeklyWorkload != nil {
		t.Fatal(v)
	}
	node["offers"] = map[string]any{"@type": "Offer", "name": "Премиальный"}
	if len(hexletLearning(data, node, c, now).Evidence()) != 0 {
		t.Fatal("another tier inherited conditions")
	}
	node["offers"] = map[string]any{"@type": "AggregateOffer", "name": "Стандартный"}
	if len(hexletLearning(data, node, c, now).Evidence()) != 0 {
		t.Fatal("minimum-price tier matched")
	}
}
