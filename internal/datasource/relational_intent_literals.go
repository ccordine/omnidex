package datasource

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	intentIntegerPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	intentDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	intentUUIDPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

func validateLiteralForColumn(literal IntentLiteral, column SchemaColumn) error {
	expected := map[ColumnTypeCategory][]LiteralType{
		TypeInteger: {LiteralInteger}, TypeDecimal: {LiteralInteger, LiteralDecimal}, TypeText: {LiteralString},
		TypeBoolean: {LiteralBoolean}, TypeTemporal: {LiteralTimestamp}, TypeDate: {LiteralDate}, TypeUUID: {LiteralUUID},
	}[column.TypeCategory]
	if len(expected) == 0 {
		return fmt.Errorf("field type %q is unsupported in filters", column.TypeCategory)
	}
	matched := false
	for _, candidate := range expected {
		matched = matched || literal.Type == candidate
	}
	if !matched {
		return fmt.Errorf("literal type %q is incompatible with field type %q", literal.Type, column.TypeCategory)
	}
	if len(column.AllowedValues) > 0 {
		allowed := false
		for _, candidate := range column.AllowedValues {
			allowed = allowed || literal.Value == candidate
		}
		if !allowed {
			return fmt.Errorf("literal %q is not an allowed value for field %q", literal.Value, column.ID)
		}
	}
	return validateLiteral(literal)
}

func validateLiteral(literal IntentLiteral) error {
	switch literal.Type {
	case LiteralString:
		if len(literal.Value) > MaxIntentStringLiteralBytes || strings.ContainsRune(literal.Value, '\x00') {
			return fmt.Errorf(
				"string literal is invalid or exceeds %d bytes",
				MaxIntentStringLiteralBytes,
			)
		}
	case LiteralInteger:
		if !intentIntegerPattern.MatchString(literal.Value) {
			return fmt.Errorf("integer literal %q is invalid", literal.Value)
		}
		if _, err := strconv.ParseInt(literal.Value, 10, 64); err != nil {
			return fmt.Errorf("integer literal is outside signed 64-bit range")
		}
	case LiteralDecimal:
		if len(literal.Value) > MaxIntentDecimalLiteralBytes || !intentDecimalPattern.MatchString(literal.Value) {
			return fmt.Errorf("decimal literal %q is invalid", literal.Value)
		}
	case LiteralBoolean:
		if literal.Value != "true" && literal.Value != "false" {
			return fmt.Errorf("boolean literal must be true or false")
		}
	case LiteralTimestamp:
		if _, err := time.Parse(time.RFC3339, literal.Value); err != nil {
			return fmt.Errorf("timestamp literal is invalid: %w", err)
		}
	case LiteralDate:
		if _, err := time.Parse("2006-01-02", literal.Value); err != nil {
			return fmt.Errorf("date literal is invalid: %w", err)
		}
	case LiteralUUID:
		if !intentUUIDPattern.MatchString(literal.Value) {
			return fmt.Errorf("UUID literal is invalid")
		}
	default:
		return fmt.Errorf("unsupported literal type %q", literal.Type)
	}
	return nil
}

func parseLiteral(literal IntentLiteral) (any, error) {
	if err := validateLiteral(literal); err != nil {
		return nil, err
	}
	switch literal.Type {
	case LiteralInteger:
		return strconv.ParseInt(literal.Value, 10, 64)
	case LiteralBoolean:
		return strconv.ParseBool(literal.Value)
	case LiteralTimestamp:
		return time.Parse(time.RFC3339, literal.Value)
	case LiteralDate:
		return time.Parse("2006-01-02", literal.Value)
	default:
		return literal.Value, nil
	}
}
