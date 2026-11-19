// Package privatejson reads bounded, private Unix configuration files.
package privatejson

import (
	"bytes"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

var ErrInvalid = errors.New("private configuration unavailable or invalid")

func Read(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	f := os.NewFile(uintptr(fd), "private-config")
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0077 != 0 || st.Size > 16384 || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(data) > 16384 {
		return nil, ErrInvalid
	}
	return data, nil
}
func unique(d *json.Decoder, depth int) bool {
	if depth > 8 {
		return false
	}
	t, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return true
	}
	if delim != '{' && delim != '[' {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		if delim == '{' {
			t, err = d.Token()
			key, ok := t.(string)
			if err != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
		}
		if !unique(d, depth+1) {
			return false
		}
	}
	end, err := d.Token()
	return err == nil && (delim == '{' && end == json.Delim('}') || delim == '[' && end == json.Delim(']'))
}
func Decode(data []byte, target any, names ...string) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if !unique(d, 0) {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != len(names) {
		return ErrInvalid
	}
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return ErrInvalid
		}
	}
	if json.Unmarshal(data, target) != nil {
		return ErrInvalid
	}
	return nil
}
