package password

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"errors"
	"sort"
	"strings"
)

//go:embed blocklist/common.sha256
var commonDigests []byte
var ErrCommonPassword = errors.New("choose a less common password")

type Policy struct{ digests []byte }

func DefaultPolicy() Policy { return Policy{digests: commonDigests} }
func (p Policy) Check(value string) error {
	if err := ValidateNewPassword(value); err != nil {
		return err
	}
	if len(p.digests) < 1000*sha256.Size || len(p.digests)%sha256.Size != 0 {
		return ErrConfiguration
	}
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	count := len(p.digests) / sha256.Size
	i := sort.Search(count, func(i int) bool { return bytes.Compare(p.digests[i*32:(i+1)*32], digest[:]) >= 0 })
	if i < count && bytes.Equal(p.digests[i*32:(i+1)*32], digest[:]) {
		return ErrCommonPassword
	}
	return nil
}

//go:embed blocklist/LICENSE
var commonLicense string

// BlocklistLicense keeps the third-party notice available in standalone binaries.
func BlocklistLicense() string { return commonLicense }
