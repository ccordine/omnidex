package omnidex

import (
	"fmt"
	"strings"
)

func validateDirectDataSource(input DirectDataSourceInput) error {
	if strings.TrimSpace(input.Name) == "" || input.Name != strings.TrimSpace(input.Name) {
		return fmt.Errorf("data-source name must be exact nonblank text")
	}
	if input.Port < 1 || input.Port > 65535 {
		return fmt.Errorf("PostgreSQL port must be between 1 and 65535")
	}
	if !validSSLMode(input.SSLMode) {
		return fmt.Errorf("PostgreSQL SSL mode %q is unsupported", input.SSLMode)
	}
	if input.UseDSN {
		if strings.TrimSpace(input.DSN) == "" || input.DSN != strings.TrimSpace(input.DSN) {
			return fmt.Errorf("PostgreSQL DSN is required in DSN mode")
		}
	} else if input.Host == "" || input.DatabaseName == "" || input.Username == "" ||
		input.Host != strings.TrimSpace(input.Host) || input.DatabaseName != strings.TrimSpace(input.DatabaseName) ||
		input.Username != strings.TrimSpace(input.Username) {
		return fmt.Errorf("PostgreSQL host, database, and username are required outside DSN mode")
	}
	return nil
}

func validateDelegatedDataSource(input DelegatedDataSourceInput) error {
	if strings.TrimSpace(input.Name) == "" || input.Name != strings.TrimSpace(input.Name) {
		return fmt.Errorf("data-source name must be exact nonblank text")
	}
	if err := validateDelegatedBaseURL(input.AuthorityURL); err != nil {
		return err
	}
	if !validEnvironmentName(input.CredentialEnv) {
		return fmt.Errorf("credential environment variable is outside the dedicated namespace OMNIDEX_DELEGATED_AUTHORITY_*")
	}
	return nil
}

func validateDelegatedBaseURL(value string) error {
	return validateClientConfiguration(value, "12345678901234567890123456789012")
}

func validateCreateChannel(input CreateChannelInput) error {
	if err := validateCanonicalID("channel ID", input.ID, 96); err != nil {
		return err
	}
	if err := validateCanonicalID("data-source ID", input.DataSourceID, 128); err != nil {
		return err
	}
	if input.Name == "" || input.Name != strings.TrimSpace(input.Name) || input.WorkspaceRoot == "" ||
		input.WorkspaceRoot != strings.TrimSpace(input.WorkspaceRoot) {
		return fmt.Errorf("channel name and workspace root must be exact nonblank text")
	}
	if len(input.Tags) > 32 {
		return fmt.Errorf("channel tags exceed 32 entries")
	}
	seen := map[string]struct{}{}
	for _, tag := range input.Tags {
		if tag == "" || tag != strings.TrimSpace(tag) || tag != strings.ToLower(tag) || len(tag) > 64 {
			return fmt.Errorf("channel tag %q is not canonical", tag)
		}
		if _, duplicate := seen[tag]; duplicate {
			return fmt.Errorf("channel tag %q is duplicated", tag)
		}
		seen[tag] = struct{}{}
	}
	return nil
}

func validEnvironmentName(value string) bool {
	const prefix = "OMNIDEX_DELEGATED_AUTHORITY_"
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, "_TOKEN") || len(value) > 128 {
		return false
	}
	suffix := strings.TrimSuffix(value[len(prefix):], "_TOKEN")
	if suffix == "" || suffix[0] < 'A' || suffix[0] > 'Z' {
		return false
	}
	for _, character := range []byte(suffix[1:]) {
		if character != '_' && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validSSLMode(value string) bool {
	switch value {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}
