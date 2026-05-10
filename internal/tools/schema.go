package tools

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

type Schema struct {
	Type                 string            `json:"type,omitempty"`
	Description          string            `json:"description,omitempty"`
	Required             []string          `json:"required,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
	Enum                 []string          `json:"enum,omitempty"`
	AdditionalProperties bool              `json:"additional_properties,omitempty"`
}

func (s Schema) Validate() error {
	schemaType := strings.TrimSpace(s.Type)
	if schemaType == "" {
		return nil
	}
	switch schemaType {
	case "object", "array", "string", "number", "integer", "boolean":
	default:
		return fmt.Errorf("unsupported schema type %q", s.Type)
	}
	for _, field := range s.Required {
		field = strings.TrimSpace(field)
		if field == "" {
			return fmt.Errorf("schema contains empty required field")
		}
	}
	if schemaType == "array" && s.Items != nil {
		if err := s.Items.Validate(); err != nil {
			return fmt.Errorf("array items: %w", err)
		}
	}
	if schemaType == "object" {
		keys := make([]string, 0, len(s.Properties))
		for key := range s.Properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("schema contains empty property name")
			}
			if err := s.Properties[key].Validate(); err != nil {
				return fmt.Errorf("property %s: %w", key, err)
			}
		}
	}
	return nil
}

func (s Schema) ValidateValue(value any) error {
	return s.validateValue(value, "$")
}

func (s Schema) validateValue(value any, path string) error {
	schemaType := strings.TrimSpace(s.Type)
	if schemaType == "" {
		return nil
	}
	switch schemaType {
	case "object":
		fields, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		for _, name := range s.Required {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := fields[name]; !ok {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for key, item := range fields {
			property, ok := s.Properties[key]
			if !ok {
				if s.AdditionalProperties {
					continue
				}
				return fmt.Errorf("%s.%s is not allowed", path, key)
			}
			if err := property.validateValue(item, path+"."+key); err != nil {
				return err
			}
		}
		return nil
	case "array":
		items, ok := sliceValues(value)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if s.Items == nil {
			return nil
		}
		for idx, item := range items {
			if err := s.Items.validateValue(item, fmt.Sprintf("%s[%d]", path, idx)); err != nil {
				return err
			}
		}
		return nil
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		if len(s.Enum) == 0 {
			return nil
		}
		for _, allowed := range s.Enum {
			if text == allowed {
				return nil
			}
		}
		return fmt.Errorf("%s must be one of %s", path, strings.Join(s.Enum, ", "))
	case "number":
		if !isNumber(value) {
			return fmt.Errorf("%s must be a number", path)
		}
		return nil
	case "integer":
		if !isInteger(value) {
			return fmt.Errorf("%s must be an integer", path)
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
		return nil
	default:
		return fmt.Errorf("unsupported schema type %q", s.Type)
	}
}

func isNumber(value any) bool {
	switch value.(type) {
	case float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func isInteger(value any) bool {
	switch number := value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return math.Trunc(float64(number)) == float64(number)
	case float64:
		return math.Trunc(number) == number
	default:
		return false
	}
}

func sliceValues(value any) ([]any, bool) {
	if values, ok := value.([]any); ok {
		return values, true
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil, false
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
	default:
		return nil, false
	}
	out := make([]any, 0, rv.Len())
	for idx := 0; idx < rv.Len(); idx++ {
		out = append(out, rv.Index(idx).Interface())
	}
	return out, true
}
