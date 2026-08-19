package roleplay

import (
	"fmt"
	"regexp"
	"strings"
)

type identityKind struct {
	name    string
	pattern *regexp.Regexp
}

var (
	worldIdentity = identityKind{
		name:    "roleplay world",
		pattern: regexp.MustCompile(`^rpw_[0-9a-f]{32}$`),
	}
	characterIdentity = identityKind{
		name:    "roleplay character",
		pattern: regexp.MustCompile(`^rpc_[0-9a-f]{32}$`),
	}
	eventIdentity = identityKind{
		name:    "roleplay canon event",
		pattern: regexp.MustCompile(`^rpe_[0-9a-f]{32}$`),
	}
	knowledgeIdentity = identityKind{
		name:    "roleplay character knowledge",
		pattern: regexp.MustCompile(`^rpk_[0-9a-f]{32}$`),
	}
	channelIdentityPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,95}$`)
)

func validateIdentity(id string, kind identityKind) error {
	if !kind.pattern.MatchString(id) {
		return fmt.Errorf("%s identity is invalid", kind.name)
	}
	return nil
}

func validateChannelID(id string) error {
	if !channelIdentityPattern.MatchString(id) {
		return fmt.Errorf("roleplay world requires an exact channel identity")
	}
	return nil
}

func validateName(name, field string) error {
	if name == "" || name != strings.TrimSpace(name) || len([]byte(name)) > 256 {
		return fmt.Errorf("%s must be 1 to 256 trimmed bytes", field)
	}
	return nil
}

func validateEventContent(content string) error {
	if content == "" || strings.TrimSpace(content) == "" || len([]byte(content)) > MaxCanonEventBytes {
		return fmt.Errorf("roleplay canon event content must be 1 to %d non-blank bytes", MaxCanonEventBytes)
	}
	return nil
}

func ValidateCanonFact(content string) error {
	return validateEventContent(content)
}

func validateProjectionLimit(limit int) error {
	if limit < 1 || limit > MaxProjectionEvents {
		return fmt.Errorf("roleplay projection limit must be between 1 and %d", MaxProjectionEvents)
	}
	return nil
}
