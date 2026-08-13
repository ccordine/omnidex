package envfile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 1 << 20

func Read(path string) (map[string]string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("environment file must be a regular file: %s", path)
	}
	if info.Size() > MaxBytes {
		return nil, fmt.Errorf("environment file exceeds %d bytes", MaxBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("environment file exceeds %d bytes", MaxBytes)
	}
	return Parse(raw)
}

func Parse(raw []byte) (map[string]string, error) {
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return nil, fmt.Errorf("environment file must be valid UTF-8 without NUL")
	}
	values := make(map[string]string)
	for lineNumber, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !validKey(key) {
			return nil, fmt.Errorf("environment line %d must be KEY=VALUE with an uppercase key", lineNumber+1)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("environment key %s is defined more than once", key)
		}
		if strings.Contains(value, "$(") || strings.ContainsRune(value, rune(96)) {
			return nil, fmt.Errorf("environment value for %s contains executable shell syntax", key)
		}
		values[key] = strings.TrimSpace(value)
	}
	return values, nil
}

func validKey(value string) bool {
	if value == "" || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}
