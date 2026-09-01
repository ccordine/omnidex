package projectroot

import (
	"fmt"
	"strings"
)

const directoryIdentityPrefix = "directory_identity_v1_"

// ValidateDirectoryIdentity accepts only identities issued by
// DirectoryIdentity. The identity is an attestation value, not a path or a
// credential.
func ValidateDirectoryIdentity(value string) error {
	digest := strings.TrimPrefix(value, directoryIdentityPrefix)
	if len(value) != len(directoryIdentityPrefix)+64 || len(digest) != 64 {
		return fmt.Errorf("directory identity is not one canonical v1 identity")
	}
	for _, character := range []byte(digest) {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return fmt.Errorf("directory identity is not one canonical v1 identity")
		}
	}
	return nil
}
