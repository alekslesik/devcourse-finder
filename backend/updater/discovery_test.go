package updater

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func TestSitemapExtractsOnlyCourseIdentities(t *testing.T) {
	xml := []byte(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://stepik.org/course/programming-54403/promo?ref=site</loc></url><url><loc>https://stepik.org/lesson/54403</loc></url><url><loc>https://attacker.example/course/4/promo</loc></url></urlset>`)
	kind, urls, err := sitemap(xml)
	if err != nil || kind != "urlset" || len(urls) != 3 {
		t.Fatal("sitemap rejected", err)
	}
	c, ok := candidateURL("stepik", urls[0], "stepik")
	if !ok || c.ExternalID != "54403" || c.URL != "https://stepik.org/course/54403/promo" {
		t.Fatal("unstable identity", c)
	}
	for _, u := range urls[1:] {
		if _, ok := candidateURL("stepik", u, "stepik"); ok {
			t.Fatal("unrelated URL accepted")
		}
	}
	for _, u := range []string{"https://user@otus.ru/lessons/python-basic", "https://otus.ru:444/lessons/python-basic", "https://otus.ru/lessons/python-basic/lesson/1", "https://otus.ru/lessons/python%2fbasic"} {
		if _, ok := candidateURL("otus", u, "otus"); ok {
			t.Fatal("unsafe identity accepted", u)
		}
	}
}
func TestSitemapGzipBoundsAndErrorPages(t *testing.T) {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	z.Write([]byte(`<urlset><url><loc>https://otus.ru/lessons/java-basic/</loc></url></urlset>`))
	z.Close()
	if _, urls, err := sitemap(b.Bytes()); err != nil || len(urls) != 1 {
		t.Fatal("gzip sitemap rejected")
	}
	for _, raw := range [][]byte{[]byte(`<html>Access denied</html>`), []byte(`<urlset></urlset><urlset/>`), []byte("bad xml")} {
		if _, _, err := sitemap(raw); err == nil {
			t.Fatal("error page accepted")
		}
	}
}
func TestIndexRejectsOtherHostsAndExerciseMaps(t *testing.T) {
	f := Feed{ID: "stepik", Adapter: "stepik", URL: "https://stepik.org/sitemap.xml"}
	children := childSitemaps(f, []string{"https://stepik.org/media/sitemaps/sitemap-course-promo-1.xml.gz", "https://stepik.org/media/sitemaps/sitemap-step-1.xml.gz", "https://localhost/map.xml", "http://stepik.org/map.xml"})
	if len(children) != 1 {
		t.Fatal("unsafe/unrelated shard accepted", children)
	}
	if validateFeed(Feed{ID: "evil", Adapter: "otus", URL: "https://169.254.169.254/sitemap.xml"}) == nil {
		t.Fatal("unapproved feed accepted")
	}
}

func TestOfficialCatalogLinksAndGzipFeedValidation(t *testing.T) {
	f := Feed{ID: "codebasics", Adapter: "codebasics", URL: "https://code-basics.com/ru", Kind: "catalog"}
	if err := validateFeed(f); err != nil {
		t.Fatal(err)
	}
	links, err := catalogLinks([]byte(`<a href="/ru/languages/python">Python</a><a href="/ru/languages/python/">duplicate</a><a href="/ru/languages/python/lessons/hello">fragment</a><a href="https://evil.example/ru/languages/java">external</a><a href="/en/languages/java">wrong locale</a>`), f)
	if err != nil || len(links) != 1 || links[0] != "https://code-basics.com/ru/languages/python" {
		t.Fatal(links, err)
	}
	if _, err = catalogLinks([]byte(`<html>Access denied</html>`), f); err == nil {
		t.Fatal("error page discovered")
	}
	if err = validateFeed(Feed{ID: "hexlet", Adapter: "hexlet", URL: "https://ru.hexlet.io/sitemaps/ru/sitemap.xml.gz"}); err != nil {
		t.Fatal(err)
	}
	f.URL = "https://code-basics.com/admin"
	if validateFeed(f) == nil {
		t.Fatal("unverified catalog path allowed")
	}
	children := childSitemaps(Feed{Adapter: "hexlet"}, []string{"https://ru.hexlet.io/sitemaps/ru/blogs.xml.gz", "https://ru.hexlet.io/sitemaps/ru/programs.xml.gz"})
	if len(children) != 1 {
		t.Fatal("blog shard selected", children)
	}
}

func TestTypeScriptCandidateUsesJavaScriptFamilyHint(t *testing.T) {
	if languageHint("typescript") != "javascript" {
		t.Fatal("supported TypeScript course excluded from discovery")
	}
}
