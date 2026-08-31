package projectroot

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	cliChatChannelIdentitySchema = "omnidex.cli-chat-channel.v2"
	cliChatChannelIDPrefix       = "cli-chat-"
)

// CLIChatChannelID derives the sole CLI bootstrap channel identity for one
// exact client path and physical directory identity.
func CLIChatChannelID(workspaceRoot, workspaceIdentity string) (model.ChannelID, error) {
	if err := model.ValidateChannelWorkspaceRoot(workspaceRoot); err != nil {
		return "", fmt.Errorf("CLI chat channel workspace root: %w", err)
	}
	if err := ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return "", fmt.Errorf("CLI chat channel workspace identity: %w", err)
	}
	digest := sha256.Sum256([]byte(
		cliChatChannelIdentitySchema + "\x00" + workspaceRoot + "\x00" + workspaceIdentity,
	))
	channelID := model.ChannelID(fmt.Sprintf("%s%x", cliChatChannelIDPrefix, digest[:]))
	if err := channelID.Validate(); err != nil {
		return "", fmt.Errorf("derived CLI chat channel identity: %w", err)
	}
	return channelID, nil
}

// IsCLIChatChannelID reports whether an ID occupies the reserved canonical
// CLI bootstrap identity namespace. It does not attest a workspace binding.
func IsCLIChatChannelID(channelID model.ChannelID) bool {
	digest := strings.TrimPrefix(string(channelID), cliChatChannelIDPrefix)
	if len(string(channelID)) != len(cliChatChannelIDPrefix)+64 || len(digest) != 64 {
		return false
	}
	for _, character := range []byte(digest) {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
