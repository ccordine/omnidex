package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

func TestNonDispatchedDispositionConsumesProjectionWithoutCountingModelUsage(t *testing.T) {
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	disposition := PolicyDispositionTrace{
		Schema: PolicyDispositionSchemaV3, Disposition: PolicyCallResultDisposition,
		ProjectionID:     "projection-1",
		ProjectionSHA256: strings.Repeat("d", 64), Budget: testStationBudget(),
		ResultStatus:               cognitionpolicy.CallResultFailed,
		FailureCode:                cognitionpolicy.CallFailureProviderIdentity,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
	}
	manifest.Trace[1].Kind = TracePolicyDisposition
	manifest.Trace[1].Payload = mustTraceJSONObject(t, disposition)
	manifest.Resources = Resources{PolicyCallsConsumed: 1}
	if _, err := prepareEpisodeManifest(manifest); err != nil {
		t.Fatalf("non-dispatched disposition did not consume its exact projection: %v", err)
	}

	manifest = validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Trace = append(manifest.Trace[:1], manifest.Trace[2:]...)
	for index := range manifest.Trace {
		manifest.Trace[index].Sequence = uint64(index + 1)
	}
	manifest.Resources = Resources{}
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("unused projection without a non-dispatched disposition was accepted")
	}
}

func TestModelCallTracePreservesIndeterminateProviderWrite(t *testing.T) {
	indeterminate := testModelCallTrace()
	indeterminate.ResultStatus = cognitionpolicy.CallResultFailed
	indeterminate.FailureCode = cognitionpolicy.CallFailureGeneration
	indeterminate.ProviderRequestDisposition = llm.ProviderRequestWriteIndeterminate
	indeterminate.ProviderResponseDisposition = llm.ProviderResponseTransportError
	indeterminate.ProviderDoneReason = ""
	indeterminate.ProviderUsagePresent = false
	indeterminate.ProviderUsage = llm.ProviderGenerationUsage{}
	indeterminate.InputTokens = 0
	indeterminate.OutputBytes = 0
	indeterminate.OutputTokens = 0
	if err := indeterminate.Validate(); err == nil {
		t.Fatal("indeterminate provider write was mislabeled as a fully dispatched model call")
	}

	notDispatched := indeterminate
	notDispatched.ProviderRequestDisposition = llm.ProviderRequestNotDispatched
	if err := notDispatched.Validate(); err == nil {
		t.Fatal("non-dispatched request was collapsed into a model-call trace")
	}

	disposition := PolicyDispositionTrace{
		Schema: PolicyDispositionSchemaV3, Disposition: PolicyCallResultDisposition,
		ProjectionID:     "projection-1",
		ProjectionSHA256: strings.Repeat("d", 64), Budget: testStationBudget(),
		ResultStatus:               cognitionpolicy.CallResultFailed,
		FailureCode:                cognitionpolicy.CallFailureGeneration,
		ProviderRequestDisposition: llm.ProviderRequestWriteIndeterminate,
	}
	if err := disposition.Validate(); err != nil {
		t.Fatalf("indeterminate provider write did not retain its exact typed disposition: %v", err)
	}

	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Trace[1].Kind = TracePolicyDisposition
	manifest.Trace[1].Payload = mustTraceJSONObject(t, disposition)
	manifest.Resources = Resources{PolicyCallsConsumed: 1}
	if _, err := prepareEpisodeManifest(manifest); err != nil {
		t.Fatalf("indeterminate write did not seal without invented model usage: %v", err)
	}
}

func TestEpisodeResourcesOrderPolicyConsumptionModelCallsAndDecisions(t *testing.T) {
	resources := Resources{PolicyCallsConsumed: 1, ModelCalls: 2}
	if err := resources.Validate(); err == nil {
		t.Fatal("model calls exceeded exact consumed policy calls")
	}
	resources = Resources{PolicyCallsConsumed: 2, ModelCalls: 1, ModelDecisions: 2}
	if err := resources.Validate(); err == nil {
		t.Fatal("model decisions exceeded fully dispatched model calls")
	}
}

func TestAbandonedPolicyCallConsumesAllowanceWithoutInventedProviderDisposition(t *testing.T) {
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	disposition := PolicyDispositionTrace{
		Schema: PolicyDispositionSchemaV3, Disposition: PolicyCallAbandonedDisposition,
		ProjectionID: "projection-1", ProjectionSHA256: strings.Repeat("d", 64),
		Budget: testStationBudget(),
	}
	manifest.Trace[1].Kind = TracePolicyDisposition
	manifest.Trace[1].Payload = mustTraceJSONObject(t, disposition)
	manifest.Resources = Resources{PolicyCallsConsumed: 1}
	if _, err := prepareEpisodeManifest(manifest); err != nil {
		t.Fatalf("abandoned call did not consume its exact projection: %v", err)
	}

	forged := disposition
	forged.ProviderRequestDisposition = llm.ProviderRequestWriteIndeterminate
	if err := forged.Validate(); err == nil {
		t.Fatal("abandonment invented a provider request disposition")
	}
}
