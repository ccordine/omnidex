package projectroot

import (
	"fmt"
	"strings"
)

// Lifecycle controls serve two explicit execution transports: server-local
// coding jobs and CLI channel jobs. The owning job decides which kind applies.
// CLI session boundaries always require ValidateClientWorkspaceIdentity.
func ValidateWorkspaceIdentity(value string) error {
	switch {
	case strings.HasPrefix(value, "client_"):
		return ValidateClientWorkspaceIdentity(value)
	case strings.HasPrefix(value, directoryIdentityPrefix):
		return ValidateDirectoryIdentity(value)
	default:
		return fmt.Errorf("workspace identity has no registered transport kind")
	}
}
