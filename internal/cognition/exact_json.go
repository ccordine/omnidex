package cognition

import "github.com/gryph/omnidex/internal/exactjson"

func ValidateUniqueJSONObject(raw []byte, subject string) error {
	return exactjson.ValidateUniqueObject(raw, subject)
}

func ValidateExactJSONFields(raw []byte, target any, subject string) error {
	return exactjson.ValidateExactFields(raw, target, subject)
}

func ValidateExactJSONObject(raw []byte, target any, subject string) error {
	return exactjson.ValidateObject(raw, target, subject)
}
