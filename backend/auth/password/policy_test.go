package password

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestOfflinePolicy(t *testing.T) {
	if fmt.Sprintf("%x", sha256.Sum256(commonDigests)) != "2028d1275f90841faedccfb337ffca4c4971b787f0b0514475633d3a468de05e" {
		t.Fatal("public blocklist artifact changed")
	}
	data, err := os.ReadFile("blocklist/known-common.txt")
	if err != nil {
		t.Fatal("public fixture unavailable")
	}
	sample := strings.TrimSpace(string(data))
	p := DefaultPolicy()
	if err := p.Check(sample); err != ErrCommonPassword {
		t.Fatal("known long common password accepted")
	}
	if err := p.Check("  " + strings.ToUpper(sample) + "  "); err != ErrCommonPassword {
		t.Fatal("common variant accepted")
	}
	if err := p.Check("a unique synthetic test passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := (Policy{}).Check("a unique synthetic test passphrase"); err != ErrConfiguration {
		t.Fatal("empty policy accepted")
	}
}
