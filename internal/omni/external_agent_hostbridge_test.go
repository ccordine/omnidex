package omni

import (
	"testing"
)

func TestUseHostBridgeExternalAgents(t *testing.T) {
	t.Setenv("OMNI_EXTERNAL_AGENT_FORCE_LOCAL", "")
	t.Setenv("HOST_AGENT_URL", "")
	if UseHostBridgeExternalAgents() {
		t.Fatal("expected false without HOST_AGENT_URL")
	}

	t.Setenv("HOST_AGENT_URL", "http://127.0.0.1:8091")
	if !UseHostBridgeExternalAgents() {
		t.Fatal("expected true when HOST_AGENT_URL is set")
	}

	t.Setenv("OMNI_EXTERNAL_AGENT_FORCE_LOCAL", "true")
	if UseHostBridgeExternalAgents() {
		t.Fatal("expected false when OMNI_EXTERNAL_AGENT_FORCE_LOCAL=true")
	}
}
