package queue

import (
	"encoding/hex"
	"fmt"
	"io"
)

const scrumMessageIdentityBytes = 16

// NewScrumMessageID reads one complete cryptographic identity from the
// supplied authority. The reader is explicit so failure and collisions remain
// testable; production callers pass crypto/rand.Reader.
func NewScrumMessageID(reader io.Reader) (string, error) {
	if reader == nil {
		return "", fmt.Errorf("generate Scrum message identity: entropy reader is required")
	}
	identity := make([]byte, scrumMessageIdentityBytes)
	if _, err := io.ReadFull(reader, identity); err != nil {
		return "", fmt.Errorf("generate cryptographic Scrum message identity: %w", err)
	}
	return "chatmsg_" + hex.EncodeToString(identity), nil
}
