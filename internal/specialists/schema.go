package specialists

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
)

func ValidatePayloadAgainstSchema(rawSchema []byte, payload any) error {
	if len(rawSchema) == 0 {
		return nil
	}
	var schema any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	normalized, err := canonicalJSONValue(payload)
	if err != nil {
		return fmt.Errorf("normalize payload: %w", err)
	}
	return validateSchemaValue(schema, normalized, "$")
}

func canonicalJSONValue(payload any) (any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateSchemaValue(schema any, value any, path string) error {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	if enums, ok := schemaMap["enum"].([]any); ok && len(enums) > 0 && !enumContains(enums, value) {
		return fmt.Errorf("%s must be one of the allowed enum values", path)
	}
	types := schemaTypeList(schemaMap["type"])
	if len(types) > 0 && !matchesAnySchemaType(types, value) {
		return fmt.Errorf("%s has invalid type", path)
	}
	if value == nil {
		return nil
	}
	switch dominantSchemaType(types, value) {
	case "object":
		objectValue, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		if err := validateObjectSchema(schemaMap, objectValue, path); err != nil {
			return err
		}
	case "array":
		listValue, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if err := validateArraySchema(schemaMap, listValue, path); err != nil {
			return err
		}
	}
	return nil
}

func validateObjectSchema(schemaMap map[string]any, value map[string]any, path string) error {
	properties, _ := schemaMap["properties"].(map[string]any)
	for _, name := range schemaStringSlice(schemaMap["required"]) {
		if _, ok := value[name]; !ok {
			return fmt.Errorf("%s.%s is required", path, name)
		}
	}
	additionalAllowed := true
	if raw, ok := schemaMap["additionalProperties"].(bool); ok {
		additionalAllowed = raw
	}
	for key, item := range value {
		propertySchema, hasProperty := properties[key]
		if hasProperty {
			if err := validateSchemaValue(propertySchema, item, path+"."+key); err != nil {
				return err
			}
			continue
		}
		if !additionalAllowed {
			return fmt.Errorf("%s.%s is not allowed", path, key)
		}
	}
	return nil
}

func validateArraySchema(schemaMap map[string]any, value []any, path string) error {
	itemSchema, ok := schemaMap["items"]
	if !ok {
		return nil
	}
	for idx, item := range value {
		if err := validateSchemaValue(itemSchema, item, fmt.Sprintf("%s[%d]", path, idx)); err != nil {
			return err
		}
	}
	return nil
}

func schemaTypeList(raw any) []string {
	switch typed := raw.(type) {
	case string:
		clean := strings.ToLower(strings.TrimSpace(typed))
		if clean == "" {
			return nil
		}
		return []string{clean}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			clean := strings.ToLower(strings.TrimSpace(text))
			if clean == "" {
				continue
			}
			out = append(out, clean)
		}
		return out
	default:
		return nil
	}
}

func matchesAnySchemaType(types []string, value any) bool {
	for _, schemaType := range types {
		if matchesSchemaType(schemaType, value) {
			return true
		}
	}
	return false
}

func dominantSchemaType(types []string, value any) string {
	for _, schemaType := range types {
		if matchesSchemaType(schemaType, value) {
			return schemaType
		}
	}
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return ""
	}
}

func matchesSchemaType(schemaType string, value any) bool {
	switch schemaType {
	case "null":
		return value == nil
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && math.Mod(number, 1) == 0
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return true
	}
}

func schemaStringSlice(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		text, ok := item.(string)
		if !ok {
			continue
		}
		clean := strings.TrimSpace(text)
		if clean == "" {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func enumContains(values []any, candidate any) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, candidate) {
			return true
		}
	}
	return false
}
