package projectroot

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Client identities distinguish equal local filesystem numbers on different
// installations. They are not credentials or a statement about file contents.
func ValidateClientIdentity(value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != value {
		return fmt.Errorf("client identity requires exactly 32 lowercase hexadecimal characters")
	}
	return nil
}

func ClientWorkspaceIdentity(clientID, directoryID string) (string, error) {
	if err := ValidateClientIdentity(clientID); err != nil {
		return "", err
	}
	if err := ValidateDirectoryIdentity(directoryID); err != nil {
		return "", err
	}
	return "client_" + clientID + "_" + directoryID, nil
}

func ParseClientWorkspaceIdentity(value string) (string, string, error) {
	if !strings.HasPrefix(value, "client_") {
		return "", "", fmt.Errorf("workspace identity requires exact client and directory identities")
	}
	clientID, directoryID, present := strings.Cut(strings.TrimPrefix(value, "client_"), "_")
	if !present {
		return "", "", fmt.Errorf("workspace identity lacks its directory identity")
	}
	if _, err := ClientWorkspaceIdentity(clientID, directoryID); err != nil {
		return "", "", err
	}
	return clientID, directoryID, nil
}

func ValidateClientWorkspaceIdentity(value string) error {
	_, _, err := ParseClientWorkspaceIdentity(value)
	return err
}
