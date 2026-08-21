package roleplay

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func newIdentity(prefix string) (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate roleplay identity: %w", err)
	}
	return prefix + hex.EncodeToString(bytes), nil
}

func NewWorldIdentity() (string, error) {
	return newIdentity("rpw_")
}

func NewCharacterIdentity() (string, error) {
	return newIdentity("rpc_")
}

func NewLibraryCharacterIdentity() (string, error) {
	return newIdentity("rpl_")
}
