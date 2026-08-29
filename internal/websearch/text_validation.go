package websearch

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

func validateFetchedBytes(name string, value []byte) error {
	if !utf8.Valid(value) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidFetchedText, name)
	}
	if bytes.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%w: %s contains NUL", ErrInvalidFetchedText, name)
	}
	return nil
}

func validateFetchedString(name, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidFetchedText, name)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%w: %s contains NUL", ErrInvalidFetchedText, name)
	}
	return nil
}
