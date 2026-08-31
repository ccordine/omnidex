package api

import (
	"fmt"
	"time"
)

func parseDurationSetting(name, raw string, minimum time.Duration) (time.Duration, error) {
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	if value < minimum {
		return 0, fmt.Errorf("%s must be at least %s, received %s", name, minimum, value)
	}
	return value, nil
}
