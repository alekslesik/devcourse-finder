package updater

import "errors"

// Only fixed adapter codes are logged; never source bodies or underlying errors.
type rejection string

func (r rejection) Error() string { return string(r) }
func (r rejection) Unwrap() error { return ErrSource }
func rejectionCode(err error, fallback string) string {
	var r rejection
	if errors.As(err, &r) {
		return string(r)
	}
	return fallback
}
