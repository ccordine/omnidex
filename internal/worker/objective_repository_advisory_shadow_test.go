package worker

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

func TestObjectiveAdvisoryOffAndShadowPreserveExactReviewAndOrdinaryResult(t *testing.T) {
	proof := advisoryProofCase{
		name: "shadow-baseline", objectiveID: "objective-shadow-baseline",
		requirement: "Does the callback require an explicit interface adapter?",
		evidence: []assemblyline.GroundedEvidenceCapsule{
			{ID: "R01", Text: "Consume accepts Callback, which requires Apply(Input)."},
			{ID: "R02", Text: "CallbackFunc implements Apply by invoking its underlying function."},
		},
		incorrect:       "Consume accepts a matching function directly.",
		correct:         "Consume requires CallbackFunc to adapt the matching function to Callback.",
		advice:          "Check the CallbackFunc adapter because a matching function does not directly satisfy the interface.",
		relevanceNeedle: "callbackfunc", provider: "proof-shadow-provider", model: "reasoner-shadow",
	}
	provider := &advisoryProofProvider{raw: proof.advice, provider: proof.provider, model: proof.model}
	shadowRuntime := newAdvisoryProofRuntime(objectiveadvisory.ModeShadow, proof, provider)
	off := runAdvisoryProofClosure(t, proof, proof.incorrect, nil)
	shadow := runAdvisoryProofClosure(t, proof, proof.incorrect, shadowRuntime)

	if !reflect.DeepEqual(off.result.Answer, shadow.result.Answer) ||
		off.result.ModelCalls != shadow.result.ModelCalls ||
		off.result.ReviewCalls != shadow.result.ReviewCalls ||
		off.result.CorrectionCalls != shadow.result.CorrectionCalls {
		t.Fatalf("shadow changed ordinary result off=%#v shadow=%#v", off.result, shadow.result)
	}
	if len(off.station.reviewInputs) != 1 || len(shadow.station.reviewInputs) != 1 ||
		!reflect.DeepEqual(off.station.reviewInputs[0], shadow.station.reviewInputs[0]) {
		t.Fatalf("shadow changed exact downstream review input off=%#v shadow=%#v",
			off.station.reviewInputs, shadow.station.reviewInputs)
	}
	offPrompt := advisoryProofReviewPrompt(t, off.station.reviewInputs[0])
	shadowPrompt := advisoryProofReviewPrompt(t, shadow.station.reviewInputs[0])
	if offPrompt != shadowPrompt || len(shadow.result.Advisory.ActiveCapsules) != 0 ||
		shadow.result.Advisory.Metrics.SelectedCapsules != 0 ||
		shadow.result.Advisory.Metrics.CandidateCapsules != 1 ||
		shadow.result.Advisory.Metrics.UnselectedChunks != 1 ||
		provider.calls != 1 || provider.modelSelectedOperations != 0 ||
		off.mutation.calls != 0 || shadow.mutation.calls != 0 {
		t.Fatalf("shadow leaked into active behavior report=%#v provider=%#v", shadow.result.Advisory, provider)
	}
	t.Logf(
		"advisory_shadow_metrics output_equal=%t review_input_equal=%t review_prompt_byte_delta=%d raw_bytes=%d chunks=%d candidate_capsules=%d selected_capsules=%d unselected_chunks=%d potential_capsule_content_bytes=%d potential_capsule_content_tokens=%d rendered_bytes=%d advisory_source_calls=%d ordinary_model_call_delta=%d mutations=%d authority_violations=%d model_selected_operations=%d",
		reflect.DeepEqual(off.result.Answer, shadow.result.Answer),
		reflect.DeepEqual(off.station.reviewInputs[0], shadow.station.reviewInputs[0]),
		len(shadowPrompt)-len(offPrompt), shadow.result.Advisory.Metrics.RawBytes,
		shadow.result.Advisory.Metrics.ChunksProduced,
		shadow.result.Advisory.Metrics.CandidateCapsules,
		shadow.result.Advisory.Metrics.SelectedCapsules,
		shadow.result.Advisory.Metrics.UnselectedChunks,
		shadow.result.Advisory.Metrics.PotentialCapsuleContentBytes,
		shadow.result.Advisory.Metrics.PotentialCapsuleContentTokens,
		shadow.result.Advisory.Projection.RenderedBytes,
		shadow.result.Advisory.Metrics.AdvisoryCalls,
		shadow.result.ModelCalls-off.result.ModelCalls,
		off.mutation.calls+shadow.mutation.calls,
		advisoryProofAuthorityViolations(proof, shadow),
		provider.modelSelectedOperations,
	)
}
