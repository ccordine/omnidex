package taskstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	// MaxJSONObjectInputBytes admits PostgreSQL's whitespace-expanded JSONB
	// text representation. NewJSONObject still rejects any compact canonical
	// object larger than MaxJSONObjectBytes.
	MaxJSONObjectInputBytes          = 128 * 1024
	MaxJSONObjectBytes               = 64 * 1024
	maxPostgresJSONBIntegerDigits    = 131072
	maxPostgresJSONBFractionalDigits = 16383
)

type JSONObject struct {
	raw json.RawMessage
}

func NewJSONObject(raw []byte) (JSONObject, error) {
	if len(raw) > MaxJSONObjectInputBytes {
		return JSONObject{}, fmt.Errorf("task state JSON input exceeds the %d-byte persistence limit", MaxJSONObjectInputBytes)
	}
	if !utf8.Valid(raw) {
		return JSONObject{}, fmt.Errorf("task state JSON object must be valid UTF-8")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return JSONObject{}, fmt.Errorf("task state JSON object requires one object value")
	}
	if err := validateJSONObjectTokens(raw); err != nil {
		return JSONObject{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return JSONObject{}, fmt.Errorf("decode task state JSON object: %w", err)
	}
	if value == nil {
		return JSONObject{}, fmt.Errorf("task state JSON object cannot be null")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return JSONObject{}, fmt.Errorf("canonicalize task state JSON object: %w", err)
	}
	if len(canonical) > MaxJSONObjectBytes {
		return JSONObject{}, fmt.Errorf("canonical task state JSON object exceeds the %d-byte limit", MaxJSONObjectBytes)
	}
	return JSONObject{raw: canonical}, nil
}

func validateJSONObjectTokens(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode task state JSON object: %w", err)
	}
	if token != json.Delim('{') {
		return fmt.Errorf("task state JSON object requires one object value")
	}
	if err := walkJSONObject(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errorsIsEOF(err) {
		if err == nil {
			return fmt.Errorf("task state JSON object has trailing data")
		}
		return fmt.Errorf("task state JSON object has invalid trailing data: %w", err)
	}
	return nil
}

func walkJSONObject(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode task state JSON object key: %w", err)
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("task state JSON object key must be a string")
		}
		if strings.ContainsRune(key, '\x00') {
			return fmt.Errorf("task state JSON object key contains PostgreSQL-forbidden NUL")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("task state JSON object contains duplicate key %q", key)
		}
		seen[key] = struct{}{}
		if err := walkJSONValue(decoder); err != nil {
			return err
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return fmt.Errorf("task state JSON object is not closed")
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode task state JSON value: %w", err)
	}
	switch value := token.(type) {
	case string:
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("task state JSON string contains PostgreSQL-forbidden NUL")
		}
	case json.Number:
		if err := validateJSONObjectNumber(value); err != nil {
			return err
		}
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return walkJSONObject(decoder)
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
			return fmt.Errorf("task state JSON array is not closed")
		}
		return nil
	default:
		return fmt.Errorf("task state JSON value has unexpected delimiter %q", delimiter)
	}
}

func validateJSONObjectNumber(number json.Number) error {
	lexeme := number.String()
	if strings.ContainsAny(lexeme, "eE") {
		return fmt.Errorf("task state JSON numbers must not use exponent notation")
	}
	negative := strings.HasPrefix(lexeme, "-")
	unsigned := strings.TrimPrefix(lexeme, "-")
	parts := strings.Split(unsigned, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || !decimalDigits(parts[0]) {
		return fmt.Errorf("task state JSON number %q is not a canonical plain decimal", lexeme)
	}
	if len(parts[0]) > maxPostgresJSONBIntegerDigits {
		return fmt.Errorf("task state JSON integer exceeds PostgreSQL JSONB numeric capacity")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" || !decimalDigits(fraction) {
			return fmt.Errorf("task state JSON number %q is not a canonical plain decimal", lexeme)
		}
		if len(fraction) > maxPostgresJSONBFractionalDigits {
			return fmt.Errorf("task state JSON fraction exceeds PostgreSQL JSONB numeric capacity")
		}
	}
	if negative && allZeroDigits(parts[0]+fraction) {
		return fmt.Errorf("task state JSON negative zero is not PostgreSQL-stable")
	}
	return nil
}

func decimalDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func allZeroDigits(value string) bool {
	for _, character := range value {
		if character != '0' {
			return false
		}
	}
	return true
}

func errorsIsEOF(err error) bool {
	return err == io.EOF
}

func EmptyJSONObject() JSONObject {
	return JSONObject{raw: json.RawMessage(`{}`)}
}

func (object JSONObject) Validate() error {
	if len(object.raw) == 0 {
		return fmt.Errorf("task state JSON object is required")
	}
	canonical, err := NewJSONObject(object.raw)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical.raw, object.raw) {
		return fmt.Errorf("task state JSON object must use canonical encoding")
	}
	return nil
}

func (object JSONObject) Bytes() []byte {
	return append([]byte(nil), object.raw...)
}

func (object JSONObject) MarshalJSON() ([]byte, error) {
	if err := object.Validate(); err != nil {
		return nil, err
	}
	return object.Bytes(), nil
}

func (object *JSONObject) UnmarshalJSON(raw []byte) error {
	if object == nil {
		return fmt.Errorf("decode task state JSON object into nil target")
	}
	decoded, err := NewJSONObject(raw)
	if err != nil {
		return err
	}
	*object = decoded
	return nil
}
