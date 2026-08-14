package worker

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

func TestObjectiveAdvisoryRescuesHandlerAdapterOmissionWithoutRegression(t *testing.T) {
	runObjectiveAdvisoryPairedProof(t, advisoryProofCase{
		name: "handler-adapter", objectiveID: "objective-handler-adapter",
		requirement: "Can Register receive a matching handler function directly, and if not, what exact adapter is required?",
		evidence: []assemblyline.GroundedEvidenceCapsule{
			{ID: "R01", Text: "type Handler interface { ServeHTTP(ResponseWriter, *Request) }; Register accepts that Handler interface."},
			{ID: "R02", Text: "type HandlerFunc func(ResponseWriter, *Request); HandlerFunc implements ServeHTTP by invoking itself."},
		},
		incorrect:       "Register accepts a handler-shaped function directly.",
		correct:         "Register requires Handler; convert a compatible function to HandlerFunc so it supplies ServeHTTP.",
		advice:          "Check the function-to-interface conversion boundary: a matching function does not implement Handler directly. The named HandlerFunc adapter supplies ServeHTTP and must wrap it.",
		relevanceNeedle: "handlerfunc", provider: "proof-local-provider", model: "reasoner-development",
	})
}

func TestObjectiveAdvisoryRescuesExclusiveResourceReasoningWithoutRegression(t *testing.T) {
	runObjectiveAdvisoryPairedProof(t, advisoryProofCase{
		name: "exclusive-resource", objectiveID: "objective-exclusive-resource",
		requirement: "What is the earliest completion time when all stated prerequisites and exclusive-resource constraints are enforced?",
		evidence: []assemblyline.GroundedEvidenceCapsule{
			{ID: "R01", Text: "Snapshot occupies the signer from time 0 through time 3; Audit then occupies the same exclusive signer for 4 time units."},
			{ID: "R02", Text: "Backup starts after Snapshot and occupies that exclusive signer for 2 units; Publish starts after Audit and Backup and occupies it for 1 unit."},
		},
		incorrect:       "The earliest completion time is 8 because Audit and Backup overlap after Snapshot.",
		correct:         "The earliest completion time is 10: Snapshot 3, serialized Audit and Backup 6, then Publish 1.",
		advice:          "Check the shared exclusive signer: Audit and Backup cannot overlap even though both become ready after Snapshot, so their durations must be serialized before Publish.",
		relevanceNeedle: "exclusive signer", provider: "proof-remote-provider", model: "reasoner-general",
	})
}

func runObjectiveAdvisoryPairedProof(t *testing.T, proof advisoryProofCase) {
	t.Helper()
	provider := &advisoryProofProvider{raw: proof.advice, provider: proof.provider, model: proof.model}
	runtime := newAdvisoryProofRuntime(objectiveadvisory.ModeActive, proof, provider)
	offFailure := runAdvisoryProofClosure(t, proof, proof.incorrect, nil)
	activeRescue := runAdvisoryProofClosure(t, proof, proof.incorrect, runtime)
	offControl := runAdvisoryProofClosure(t, proof, proof.correct, nil)
	activeControl := runAdvisoryProofClosure(t, proof, proof.correct, runtime)

	offSuccesses := boolInt(offFailure.result.Answer.Text == proof.correct) +
		boolInt(offControl.result.Answer.Text == proof.correct)
	activeSuccesses := boolInt(activeRescue.result.Answer.Text == proof.correct) +
		boolInt(activeControl.result.Answer.Text == proof.correct)
	rescues := boolInt(offFailure.result.Answer.Text != proof.correct && activeRescue.result.Answer.Text == proof.correct)
	regressions := boolInt(offControl.result.Answer.Text == proof.correct && activeControl.result.Answer.Text != proof.correct)
	if offSuccesses != 1 || activeSuccesses != 2 || rescues != 1 || regressions != 0 {
		t.Fatalf("paired result off=%d active=%d rescues=%d regressions=%d", offSuccesses, activeSuccesses, rescues, regressions)
	}
	assertActiveAdvisoryProofBoundary(t, proof, activeRescue, activeControl, provider)

	offPrompt := advisoryProofReviewPrompt(t, offFailure.station.reviewInputs[0])
	activePrompt := advisoryProofReviewPrompt(t, activeRescue.station.reviewInputs[0])
	report := activeRescue.result.Advisory
	authorityViolations := advisoryProofAuthorityViolations(proof, activeRescue) +
		advisoryProofAuthorityViolations(proof, activeControl)
	t.Logf(
		"advisory_proof_metrics case=%s off_successes=%d/2 active_successes=%d/2 rescues=%d regressions=%d raw_bytes=%d chunks=%d candidate_capsules=%d selected_capsules=%d unused_capsules=%d capsule_bytes=%d capsule_tokens=%d rendered_bytes=%d review_prompt_byte_delta=%d advisory_source_calls=%d ordinary_model_call_delta_rescue=%d ordinary_model_call_delta_control=%d mutation_calls_before_detection=%d authority_violations=%d model_selected_operations=%d",
		proof.name, offSuccesses, activeSuccesses, rescues, regressions,
		report.Metrics.RawBytes, report.Metrics.ChunksProduced, report.Metrics.CandidateCapsules,
		report.Metrics.SelectedCapsules, report.Metrics.UnselectedChunks,
		report.Metrics.SelectedCapsuleContentBytes, report.Metrics.SelectedCapsuleContentTokens,
		report.Projection.RenderedBytes,
		len(activePrompt)-len(offPrompt), report.Metrics.AdvisoryCalls,
		activeRescue.result.ModelCalls-offFailure.result.ModelCalls,
		activeControl.result.ModelCalls-offControl.result.ModelCalls,
		activeRescue.station.mutationCallsAtDetection, authorityViolations,
		provider.modelSelectedOperations,
	)
}

func runAdvisoryProofClosure(
	t *testing.T, proof advisoryProofCase, answer string, runner objectiveAdvisoryRunner,
) advisoryProofRun {
	t.Helper()
	mutation := &advisoryProofMutationSpy{}
	station := &advisoryProofStation{
		answerText: answer, incorrect: proof.incorrect, corrected: proof.correct,
		relevanceNeedle: proof.relevanceNeedle, mutation: mutation,
		mutationCallsAtDetection: -1,
	}
	result, err := runObjectiveRepositoryGroundedClosure(context.Background(), assemblyline.GroundedAnswerInput{
		RequirementID: "requirement-" + proof.name, ExactRequirement: proof.requirement,
		Evidence: append([]assemblyline.GroundedEvidenceCapsule(nil), proof.evidence...),
	}, station, objectiveRepositoryGroundedClosureOptions{
		ObjectiveID: proof.objectiveID, Generation: 1, Advisory: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	return advisoryProofRun{result: result, station: station, mutation: mutation}
}

func assertActiveAdvisoryProofBoundary(
	t *testing.T, proof advisoryProofCase,
	rescue, control advisoryProofRun,
	provider *advisoryProofProvider,
) {
	t.Helper()
	for label, run := range map[string]advisoryProofRun{"rescue": rescue, "control": control} {
		if run.result.Advisory.Metrics.SelectedCapsules != 1 || len(run.result.Advisory.ActiveCapsules) != 1 ||
			run.mutation.calls != 0 {
			t.Fatalf("%s advisory boundary metrics/report=%#v mutations=%d", label, run.result.Advisory, run.mutation.calls)
		}
	}
	if strings.Join(rescue.station.events, ",") != "answer,review,correct,review" ||
		len(rescue.station.reviewInputs) != 2 || len(rescue.station.correctionInputs) != 1 ||
		rescue.station.mutationCallsAtDetection != 0 {
		t.Fatalf("advice was not detected before correction/mutation: %#v", rescue.station)
	}
	for _, review := range rescue.station.reviewInputs {
		if len(review.AdvisoryCapsules) != 1 || review.AdvisoryCapsules[0].ID != rescue.result.Advisory.ActiveCapsules[0].ID {
			t.Fatalf("first review or re-review lost selected capsule: %#v", rescue.station.reviewInputs)
		}
	}
	if rescue.station.correctionInputs[0].Issue.Detail == "" ||
		!reflect.DeepEqual(rescue.result.Answer.EvidenceIDs, rescue.station.correctionInputs[0].EvidenceIDs) ||
		provider.calls != 2 || provider.modelSelectedOperations != 0 {
		t.Fatalf("correction/source authority escaped code control: provider=%#v correction=%#v", provider, rescue.station.correctionInputs)
	}
	if rescue.result.Advisory.Metrics.RawBytes != len([]byte(proof.advice)) {
		t.Fatalf("raw byte accounting=%d want=%d", rescue.result.Advisory.Metrics.RawBytes, len([]byte(proof.advice)))
	}
}

func advisoryProofReviewPrompt(t *testing.T, input assemblyline.RepositoryGroundedReviewInput) string {
	t.Helper()
	prompt, err := assemblyline.BuildRepositoryGroundedReviewPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	return prompt
}

func advisoryProofAuthorityViolations(proof advisoryProofCase, run advisoryProofRun) int {
	violations := 0
	for _, artifact := range run.result.Advisory.Artifacts {
		violations += boolInt(artifact.Authority != objectiveadvisory.AuthorityNonAuthoritative)
	}
	for _, capsule := range append(run.result.Advisory.CandidateCapsules, run.result.Advisory.ActiveCapsules...) {
		violations += boolInt(capsule.ValidateFor(proof.objectiveID, 1) != nil)
		for _, evidence := range proof.evidence {
			violations += boolInt(capsule.ID == evidence.ID)
		}
	}
	return violations
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
