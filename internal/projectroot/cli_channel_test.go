package projectroot

import (
	"strings"
	"testing"
)

func TestCLIChatChannelIDBindsExactRootAndPhysicalIdentity(t *testing.T) {
	t.Parallel()

	identityA := "directory_identity_v1_" + strings.Repeat("a", 64)
	identityB := "directory_identity_v1_" + strings.Repeat("b", 64)
	channelID, err := CLIChatChannelID("/tmp/exact-cli-channel", identityA)
	if err != nil {
		t.Fatalf("derive exact CLI channel ID: %v", err)
	}
	if want := "cli-chat-b6131ee3ca63b9a5454ab88b91bf82f7715b3d61dc7aadd77daa3f845fbe804f"; string(channelID) != want {
		t.Fatalf("channel ID = %q, want %q", channelID, want)
	}
	if !IsCLIChatChannelID(channelID) {
		t.Fatalf("derived channel ID %q is not in the CLI namespace", channelID)
	}

	changedRoot, err := CLIChatChannelID("/tmp/other-cli-channel", identityA)
	if err != nil {
		t.Fatalf("derive changed-root CLI channel ID: %v", err)
	}
	changedIdentity, err := CLIChatChannelID("/tmp/exact-cli-channel", identityB)
	if err != nil {
		t.Fatalf("derive changed-identity CLI channel ID: %v", err)
	}
	if changedRoot == channelID || changedIdentity == channelID || changedRoot == changedIdentity {
		t.Fatalf(
			"exact bindings collided: base=%q changed root=%q changed identity=%q",
			channelID,
			changedRoot,
			changedIdentity,
		)
	}
}
