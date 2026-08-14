package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestQwen35ProfileAcceptsClosedEquivalentParameterOrderVariants(t *testing.T) {
	t.Parallel()
	reordered := strings.Replace(
		exactProviderQwen35Show,
		"temperature                    1\\ntop_k                          20\\ntop_p                          0.95\\npresence_penalty               1.5",
		"top_k                          20\\ntop_p                          0.95\\npresence_penalty               1.5\\ntemperature                    1",
		1,
	)
	shows := []struct {
		model string
		show  string
	}{
		{model: "opaque-planner-alpha:local", show: exactProviderQwen35Show},
		{model: "unrelated-reviewer-beta:local", show: reordered},
	}

	if got, want := exactParameterAssignments(t, shows[0].show), exactParameterAssignments(t, shows[1].show); !reflect.DeepEqual(got, want) {
		t.Fatalf("closed parameter variants are not behaviorally equivalent: got=%v want=%v", got, want)
	}

	var wantTransport ExactPreparedTransportSettings
	for index, fixture := range shows {
		selection, evidence := exactProviderProfileEvidence(t, fixture.model, fixture.show)
		identity, err := DeriveExactProviderIdentityExpectation(evidence, selection)
		if err != nil {
			t.Fatalf("derive closed qwen35 variant %d: %v", index, err)
		}
		if identity.Model != fixture.model || identity.TokenizerProfile != ExactPreparedTokenizerProfile {
			t.Fatalf("variant %d identity=%+v", index, identity)
		}
		transport, err := ResolveExactPreparedTransport(identity)
		if err != nil {
			t.Fatalf("resolve closed qwen35 variant %d transport: %v", index, err)
		}
		if index == 0 {
			wantTransport = transport
		} else if !reflect.DeepEqual(transport, wantTransport) {
			t.Fatalf("variant %d transport=%+v want=%+v", index, transport, wantTransport)
		}
	}

	unregisteredOrder := strings.Replace(
		exactProviderQwen35Show,
		"temperature                    1\\ntop_k                          20\\ntop_p                          0.95\\npresence_penalty               1.5",
		"temperature                    1\\ntop_p                          0.95\\ntop_k                          20\\npresence_penalty               1.5",
		1,
	)
	selection, evidence := exactProviderProfileEvidence(
		t, "third-opaque-identity:local", unregisteredOrder,
	)
	if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
		t.Fatal("unregistered parameter ordering was accepted outside the closed profile")
	}
}

func exactParameterAssignments(t *testing.T, show string) map[string]string {
	t.Helper()
	var response struct {
		Parameters string `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(show), &response); err != nil {
		t.Fatal(err)
	}
	assignments := make(map[string]string)
	for _, line := range strings.Split(response.Parameters, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("parameter line %q is not one exact assignment", line)
		}
		if _, exists := assignments[fields[0]]; exists {
			t.Fatalf("parameter %q is duplicated", fields[0])
		}
		assignments[fields[0]] = fields[1]
	}
	return assignments
}
