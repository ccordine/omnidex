package roleplay

import (
	"errors"
	"testing"
)

func TestWebResearchCapabilityIsOneClosedCodeOwnedValue(t *testing.T) {
	t.Parallel()
	worldID := "rpw_11111111111111111111111111111111"
	characterID := "rpc_22222222222222222222222222222222"
	if err := validateResearchCapabilityIdentity(worldID, characterID, CapabilityWebResearch); err != nil {
		t.Fatal(err)
	}
	if err := validateResearchCapabilityIdentity(worldID, characterID, "browse"); err == nil {
		t.Fatal("unregistered character capability was accepted")
	}
	if !errors.Is(ErrResearchCapabilityDenied, ErrResearchCapabilityDenied) {
		t.Fatal("research capability denial lost its typed identity")
	}
}
