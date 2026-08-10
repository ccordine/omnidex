package exactjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// ValidateUniqueObject rejects repeated authority fields before Go's JSON
// decoder can silently select one value.
func ValidateUniqueObject(raw []byte, subject string) error {
	if subject == "" {
		return fmt.Errorf("exact JSON contract is uninitialized")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil {
		return err
	}
	if start != json.Delim('{') {
		return fmt.Errorf("%s must be one JSON object", subject)
	}
	if err := walkUniqueObject(decoder); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s has trailing JSON token %v", subject, token)
	}
	return nil
}

// ValidateExactFields rejects unknown fields and the case-insensitive aliases
// accepted by encoding/json.
func ValidateExactFields(raw []byte, target any, subject string) error {
	if target == nil || subject == "" {
		return fmt.Errorf("exact JSON contract is uninitialized")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return validateShape(value, reflect.TypeOf(target), subject, false)
}

func ValidateObject(raw []byte, target any, subject string) error {
	if err := ValidateUniqueObject(raw, subject); err != nil {
		return err
	}
	return ValidateExactFields(raw, target, subject)
}

// ValidateCompatibleObject permits unrelated provider metadata while rejecting
// duplicate keys and inexact case aliases of every known typed field.
func ValidateCompatibleObject(raw []byte, target any, subject string) error {
	if err := ValidateUniqueObject(raw, subject); err != nil {
		return err
	}
	if target == nil || subject == "" {
		return fmt.Errorf("exact JSON contract is uninitialized")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return validateShape(value, reflect.TypeOf(target), subject, true)
}

func validateShape(value any, target reflect.Type, path string, allowUnknown bool) error {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if value == nil {
		return nil
	}
	unmarshaler := reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	if target.Implements(unmarshaler) ||
		(target.Kind() != reflect.Pointer && reflect.PointerTo(target).Implements(unmarshaler)) {
		return nil
	}
	switch target.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		fields := make(map[string]reflect.Type, target.NumField())
		for index := 0; index < target.NumField(); index++ {
			field := target.Field(index)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
		for name, nested := range object {
			fieldType, exists := fields[name]
			if !exists {
				if allowUnknown {
					for registered := range fields {
						if strings.EqualFold(name, registered) {
							return fmt.Errorf("%s contains inexact alias %q for %q", path, name, registered)
						}
					}
					continue
				}
				return fmt.Errorf("%s contains inexact or unknown field %q", path, name)
			}
			if err := validateShape(nested, fieldType, path+"."+name, allowUnknown); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		values, ok := value.([]any)
		if !ok {
			return nil
		}
		for index, nested := range values {
			if err := validateShape(nested, target.Elem(), fmt.Sprintf("%s[%d]", path, index), allowUnknown); err != nil {
				return err
			}
		}
	}
	return nil
}

func walkUniqueObject(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("JSON object key is not a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("JSON object contains duplicate key %q", key)
		}
		seen[key] = struct{}{}
		if err := walkValue(decoder); err != nil {
			return err
		}
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		if err != nil {
			return err
		}
		return fmt.Errorf("JSON object is not closed")
	}
	return nil
}

func walkValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, nested := token.(json.Delim)
	if !nested {
		return nil
	}
	switch delimiter {
	case '{':
		return walkUniqueObject(decoder)
	case '[':
		for decoder.More() {
			if err := walkValue(decoder); err != nil {
				return err
			}
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
			if err != nil {
				return err
			}
			return fmt.Errorf("JSON array is not closed")
		}
		return nil
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}
