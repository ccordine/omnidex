package codex

import (
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentRoleOptionsReachNativeRequestWithoutModelSubstitution(t *testing.T) {
	for _, effort := range []string{"low", "high", "xhigh", "max", "ultra", "future-supported-option"} {
		parameters := map[string]any{"model": "chosen-model"}
		applyAgentInferenceOptions(parameters, codingagent.Route{Effort: effort, ServiceTier: "priority"})
		if parameters["effort"] != effort || parameters["serviceTier"] != "priority" || parameters["model"] != "chosen-model" {
			t.Fatal("native options changed the selected role")
		}
	}
	parameters := map[string]any{}
	applyAgentInferenceOptions(parameters, codingagent.Route{})
	if _, present := parameters["effort"]; present || parameters["serviceTier"] != "default" {
		t.Fatal("native defaults replaced explicit standard speed or model-default effort")
	}
}
