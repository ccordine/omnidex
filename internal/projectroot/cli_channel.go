package projectroot

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const CLIChatChannelIDPrefix = "cli-chat-"

// NewCLIChatChannelID allocates an opaque identity. The repository stores and
// resolves the actual workspace binding; no client reconstructs it from this ID.
func NewCLIChatChannelID() (model.ChannelID, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("allocate CLI chat channel identity: %w", err)
	}
	return model.ChannelID(CLIChatChannelIDPrefix + hex.EncodeToString(nonce[:])), nil
}

// IsCLIChatChannelID reports whether an ID occupies the reserved canonical
// CLI bootstrap identity namespace. It does not attest a workspace binding.
func IsCLIChatChannelID(channelID model.ChannelID) bool {
	nonce, found := strings.CutPrefix(string(channelID), CLIChatChannelIDPrefix)
	if !found || len(nonce) != 32 {
		return false
	}
	for _, character := range []byte(nonce) {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
