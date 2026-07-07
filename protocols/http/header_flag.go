package http

import (
	"fmt"
	"strings"
)

// headerFlag implements flag.Value for repeated --header flags.
type headerFlag struct {
	headers []Header
}

func (f *headerFlag) String() string { return "" }

func (f *headerFlag) Set(s string) error {
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return fmt.Errorf("header %q must be in Name:Value form", s)
	}
	f.headers = append(f.headers, Header{
		Name:  strings.TrimSpace(s[:idx]),
		Value: strings.TrimSpace(s[idx+1:]),
	})
	return nil
}

func (f *headerFlag) Get() any { return f.headers }
