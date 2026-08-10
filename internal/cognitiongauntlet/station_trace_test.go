package cognitiongauntlet

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestEpisodeSealRejectsUnstructuredOrMismatchedStationTrace(t *testing.T) {
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Trace[0].Payload = terminalTestPayload(t)
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted an unstructured Context Projection trace")
	}

	manifest = validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	call := testModelCallTrace()
	call.ProjectionSHA256 = strings.Repeat("f", 64)
	manifest.Trace[1].Payload = mustTraceJSONObject(t, call)
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted a model call bound to another Context Projection")
	}

	manifest = validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	call = testModelCallTrace()
	call.Budget.MaxInputBytes++
	manifest.Trace[1].Payload = mustTraceJSONObject(t, call)
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted a per-call station budget outside sealed authority")
	}
}

func TestEpisodeSealReconcilesStationUsageWithResources(t *testing.T) {
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Resources.ContextBytes++
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted aggregate context bytes that differ from sealed calls")
	}
	manifest = validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Resources.OutputTokens++
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted aggregate output tokens that differ from sealed calls")
	}
}

func TestProjectionTraceRejectsSelfSourceLineage(t *testing.T) {
	projection := testProjectionTrace()
	projection.Selected = projection.Selected[:1]
	projection.Selected[0].SourceRefs = []ProjectionReferenceIdentity{projection.Selected[0].Ref}
	if err := projection.Validate(); err == nil {
		t.Fatal("raw projected evidence accepted itself as derived source lineage")
	}
}

func TestModelCallTraceSealsExactNativeUsageLimitWithoutClaimingQualification(t *testing.T) {
	call := testModelCallTrace()
	call.ResultStatus = cognitionpolicy.CallResultRejected
	call.FailureCode = cognitionpolicy.CallFailureProviderUsageLimit
	call.ProviderUsage.PromptEvalCount = call.Budget.MaxInputTokens + 1
	call.InputTokens = int64(call.ProviderUsage.PromptEvalCount)
	if err := call.Validate(); err != nil {
		t.Fatalf("exact usage-limit rejection did not seal: %v", err)
	}
	if callBudgetQualified(call) {
		t.Fatal("over-budget provider call was competence-qualified")
	}

	forged := call
	forged.ResultStatus = cognitionpolicy.CallResultAccepted
	forged.FailureCode = ""
	if err := forged.Validate(); err == nil {
		t.Fatal("over-budget accepted call was admitted")
	}
	forged = call
	forged.FailureCode = cognitionpolicy.CallFailureInvalidDecision
	if err := forged.Validate(); err == nil {
		t.Fatal("over-budget call without exact usage-limit attribution was admitted")
	}
}

func TestNonDispatchedDispositionConsumesProjectionWithoutCountingModelUsage(t *testing.T) {
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	disposition := PolicyDispositionTrace{
		Schema: PolicyDispositionSchemaV1, ProjectionID: "projection-1",
		ProjectionSHA256: strings.Repeat("d", 64), Budget: testStationBudget(),
		ResultStatus:              cognitionpolicy.CallResultFailed,
		FailureCode:               cognitionpolicy.CallFailureProviderIdentity,
		ProviderRequestDispatched: false,
	}
	manifest.Trace[1].Kind = TracePolicyDisposition
	manifest.Trace[1].Payload = mustTraceJSONObject(t, disposition)
	manifest.Resources = Resources{}
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

func TestModelCallTraceBindsProviderDispatchAndDoneReason(t *testing.T) {
	length := testModelCallTrace()
	length.ResultStatus = cognitionpolicy.CallResultRejected
	length.FailureCode = cognitionpolicy.CallFailureResponseLimit
	length.ProviderDoneReason = "length"
	length.ProviderUsage.EvalCount = length.Budget.MaxOutputTokens
	length.OutputTokens = int64(length.Budget.MaxOutputTokens)
	if err := length.Validate(); err != nil {
		t.Fatalf("exact provider length rejection did not seal: %v", err)
	}

	acceptedLength := length
	acceptedLength.ResultStatus = cognitionpolicy.CallResultAccepted
	acceptedLength.FailureCode = ""
	if err := acceptedLength.Validate(); err == nil {
		t.Fatal("accepted model call claimed a provider length stop")
	}

	transport := testModelCallTrace()
	transport.ResultStatus = cognitionpolicy.CallResultFailed
	transport.FailureCode = cognitionpolicy.CallFailureGeneration
	transport.ProviderResponseDisposition = llm.ProviderResponseTransportError
	transport.ProviderDoneReason = ""
	transport.ProviderUsagePresent = false
	transport.ProviderUsage = llm.ProviderGenerationUsage{}
	transport.InputTokens, transport.OutputTokens, transport.OutputBytes = 0, 0, 0
	if err := transport.Validate(); err != nil {
		t.Fatalf("dispatched transport failure did not seal without claiming native inference: %v", err)
	}
}

func sealedModelEpisode(t *testing.T, selected []ProjectedReference) SealedEpisode {
	t.Helper()
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	projection := testProjectionTrace()
	projection.Selected = append([]ProjectedReference(nil), selected...)
	projection.RenderedBytes = 80
	manifest.Trace[0].Payload = mustTraceJSONObject(t, projection)
	path := filepath.Join(t.TempDir(), "episode.json")
	sealed, err := SealEpisode(path, manifest)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func testStationBudget() StationBudget {
	return StationBudget{
		MaxInputBytes: 4096, MaxInputTokens: 1024,
		MaxOutputBytes: 1024, MaxOutputTokens: 256,
	}
}

func testProjectionTrace() ProjectionTrace {
	return ProjectionTrace{
		Schema: ProjectionTraceSchemaV1, ProjectionID: "projection-1",
		ProjectionSHA256: strings.Repeat("d", 64), RenderedBytes: 80,
		EstimatedTokens: 20, TokenEstimator: "utf8-bytes-div-four.v1",
		Selected: []ProjectedReference{
			projectedReference("evidence://critical", 40, "a"),
			projectedReference("evidence://distractor", 20, "b"),
		},
	}
}

func testModelCallTrace() ModelCallTrace {
	return ModelCallTrace{
		Schema: ModelCallTraceSchemaV2, ProjectionID: "projection-1",
		ProjectionSHA256: strings.Repeat("d", 64), Budget: testStationBudget(),
		ResultStatus:                cognitionpolicy.CallResultAccepted,
		ProviderResponseDisposition: llm.ProviderResponseSucceeded,
		ProviderRequestDispatched:   true, ProviderDoneReason: "stop", ProviderUsagePresent: true,
		ProviderUsage: llm.ProviderGenerationUsage{
			PromptEvalCount: 32, EvalCount: 16, TotalDurationNanos: 4,
			LoadDurationNanos: 1, PromptEvalDurationNanos: 1, EvalDurationNanos: 1,
		},
		InputBytes: 128, InputTokens: 32, OutputBytes: 64, OutputTokens: 16,
	}
}

func testProjectionRelevance(
	episode SealedEpisode,
	oracle OracleManifest,
) *ProjectionRelevanceEvidence {
	return &ProjectionRelevanceEvidence{
		Schema: ProjectionRelevanceSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256,
		RelevantRefs: []ProjectionReferenceIdentity{
			projectionReferenceIdentity("evidence://critical", "a"),
		},
		CriticalUses: []CriticalProjectionUse{{
			ProjectionID:  "projection-1",
			Ref:           projectionReferenceIdentity("evidence://critical", "a"),
			RequiredBytes: 40,
		}},
	}
}

func terminalTestPayload(t *testing.T) taskstate.JSONObject {
	t.Helper()
	return mustTraceJSONObject(t, struct {
		Kind string `json:"kind"`
	}{Kind: "terminal"})
}

func mustTraceJSONObject(t *testing.T, value any) taskstate.JSONObject {
	t.Helper()
	payload, err := traceJSONObject(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
