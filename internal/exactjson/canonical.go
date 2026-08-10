package exactjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Canonical marshals the JSON value with UTF-8 key ordering and no
// insignificant whitespace. It matches cognition_canonical_jsonb in the
// PostgreSQL authority schema.
func Canonical(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal exact JSON authority: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode exact JSON authority: %w", err)
	}
	var rendered strings.Builder
	if err := writeCanonical(&rendered, decoded); err != nil {
		return nil, err
	}
	return []byte(rendered.String()), nil
}

func writeCanonical(rendered *strings.Builder, value any) error {
	switch typed := value.(type) {
	case nil:
		rendered.WriteString("null")
	case bool:
		if typed {
			rendered.WriteString("true")
		} else {
			rendered.WriteString("false")
		}
	case json.Number:
		rendered.WriteString(typed.String())
	case string:
		writeCanonicalString(rendered, typed)
	case []any:
		rendered.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				rendered.WriteByte(',')
			}
			if err := writeCanonical(rendered, item); err != nil {
				return err
			}
		}
		rendered.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		rendered.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				rendered.WriteByte(',')
			}
			writeCanonicalString(rendered, key)
			rendered.WriteByte(':')
			if err := writeCanonical(rendered, typed[key]); err != nil {
				return err
			}
		}
		rendered.WriteByte('}')
	default:
		return fmt.Errorf("exact JSON authority contains unsupported value %T", value)
	}
	return nil
}

func writeCanonicalString(rendered *strings.Builder, value string) {
	const hex = "0123456789abcdef"
	rendered.WriteByte('"')
	for len(value) > 0 {
		runeValue, size := utf8.DecodeRuneInString(value)
		if runeValue == utf8.RuneError && size == 1 {
			runeValue = utf8.RuneError
		}
		value = value[size:]
		switch runeValue {
		case '"', '\\':
			rendered.WriteByte('\\')
			rendered.WriteRune(runeValue)
		case '\b':
			rendered.WriteString(`\b`)
		case '\f':
			rendered.WriteString(`\f`)
		case '\n':
			rendered.WriteString(`\n`)
		case '\r':
			rendered.WriteString(`\r`)
		case '\t':
			rendered.WriteString(`\t`)
		default:
			if runeValue < 0x20 {
				rendered.WriteString(`\u00`)
				rendered.WriteByte(hex[byte(runeValue)>>4])
				rendered.WriteByte(hex[byte(runeValue)&0x0f])
			} else {
				rendered.WriteRune(runeValue)
			}
		}
	}
	rendered.WriteByte('"')
}
