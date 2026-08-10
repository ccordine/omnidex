package cognitiongauntlet

import (
	"fmt"
	"net"
	"net/url"
	"time"
)

func (ready hostProcessReady) Validate(config hostProcessConfig) error {
	if ready.Schema != hostProcessReadySchemaV1 || ready.PID <= 0 ||
		ready.CurrentRole != config.ExpectedRole || ready.Scenario != config.Scenario ||
		ready.Scenario.Validate() != nil ||
		ready.StartedAt.IsZero() || ready.StartedAt.After(time.Now().UTC().Add(time.Minute)) {
		return fmt.Errorf("offline host readiness authority is invalid")
	}
	parsed, err := url.Parse(ready.BaseURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() == "" ||
		!net.ParseIP(parsed.Hostname()).IsLoopback() {
		return fmt.Errorf("offline host readiness URL is not an exact loopback endpoint")
	}
	return nil
}

func loadHostProcessReady(path string, config hostProcessConfig) (hostProcessReady, error) {
	var ready hostProcessReady
	if err := loadStrictJSONFile(path, &ready, "offline host readiness"); err != nil {
		return hostProcessReady{}, err
	}
	if err := ready.Validate(config); err != nil {
		return hostProcessReady{}, err
	}
	return ready, nil
}
