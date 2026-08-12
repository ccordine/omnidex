package llm

import (
	"fmt"
	"time"
)

func parseExactProviderTimestamp(raw string, maxFractionDigits int) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || parsed.Format(time.RFC3339Nano) != raw {
		return time.Time{}, fmt.Errorf("provider timestamp is not canonical RFC3339 UTC")
	}
	if err := validateExactProviderTimestamp(parsed, maxFractionDigits); err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func validateExactProviderTimestamp(value time.Time, maxFractionDigits int) error {
	if maxFractionDigits < 0 || maxFractionDigits > 9 {
		return fmt.Errorf("provider timestamp precision authority is invalid")
	}
	if value.Location() != time.UTC || value.Year() < 1 || value.Year() > 9999 {
		return fmt.Errorf("provider timestamp is outside canonical PostgreSQL UTC authority")
	}
	precision := 1
	for digits := maxFractionDigits; digits < 9; digits++ {
		precision *= 10
	}
	if value.Nanosecond()%precision != 0 {
		return fmt.Errorf("provider timestamp exceeds its registered fractional precision")
	}
	return nil
}
