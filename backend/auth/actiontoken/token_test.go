package actiontoken

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSecretAndOrigin(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Generate()
	if a.Encoded() == b.Encoded() {
		t.Fatal("reused secret")
	}
	parsed, err := Parse(a.Encoded())
	if err != nil || parsed != a {
		t.Fatal("roundtrip failed")
	}
	x, _ := a.Digest(ConfirmEmail)
	y, _ := a.Digest(ResetPassword)
	if x == y {
		t.Fatal("purpose domains collide")
	}
	for _, bad := range []string{"", a.Encoded() + "=", "v2." + a.Encoded()[3:], strings.Repeat("x", 1000)} {
		if _, err := Parse(bad); err == nil {
			t.Fatal("unsafe token accepted")
		}
	}
	for _, origin := range []string{"http://example.test", "https://user:password@example.test", "https://example.test/path", "https://example.test?host=evil", "https://example.test#x", "https://example.test:444", "https://evil\\example.test", "https://example.test?", "https://Example.test"} {
		if _, err := NewOrigin(origin); err == nil {
			t.Fatal("untrusted origin accepted")
		}
	}
	origin, err := NewOrigin("https://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	link, err := origin.Link(ResetPassword, a)
	if err != nil || !strings.HasPrefix(link, "https://example.test/account/reset-password#token=v1.") {
		t.Fatal("incorrect fragment link")
	}
	encoded, _ := json.Marshal(a)
	if string(encoded) != "{}" {
		t.Fatal("token JSON leaked")
	}
	for _, out := range []string{fmt.Sprintf("%+v", a), fmt.Sprintf("%#v", a)} {
		if strings.Contains(out, a.Encoded()) || !strings.Contains(out, "redacted") {
			t.Fatal("token formatted")
		}
	}
}
