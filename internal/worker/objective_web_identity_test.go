package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

func TestWebReviewRoutesMustBeConfiguredAndDistinct(t *testing.T) {
	routing := ModelRouting{Stations: map[station.ID]string{
		station.WebGroundedSynthesis:           "synthesis-model",
		station.WebGroundedSynthesisCorrection: "synthesis-model",
		station.WebClaimEvidenceReview:         "synthesis-model",
	}}
	if err := requireIndependentWebReviewRoutes(routing); err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("equal synthesis/review route error=%v", err)
	}
	routing.Stations[station.WebClaimEvidenceReview] = "review-model"
	if err := requireIndependentWebReviewRoutes(routing); err != nil {
		t.Fatal(err)
	}
	routing.Stations[station.WebGroundedSynthesisCorrection] = "review-model"
	if err := requireIndependentWebReviewRoutes(routing); err == nil || !strings.Contains(err.Error(), "synthesis correction") {
		t.Fatalf("equal correction/review route error=%v", err)
	}
	routing.Stations[station.WebGroundedSynthesisCorrection] = "synthesis-model"
	delete(routing.Stations, station.WebClaimEvidenceReview)
	if err := requireIndependentWebReviewRoutes(routing); err == nil || !strings.Contains(err.Error(), "no configured model") {
		t.Fatalf("missing lazy review route error=%v", err)
	}
}

func TestWebReviewExactIdentityMustDifferFromSynthesis(t *testing.T) {
	identity := webIdentityFixture("synthesis", strings.Repeat("a", 64))
	guard := &webModelIdentityGuard{}
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkWebGroundedSynthesis},
		exactStationExecution{ProviderIdentity: identity},
	); err != nil {
		t.Fatal(err)
	}
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkWebClaimEvidenceReview},
		exactStationExecution{ProviderIdentity: identity},
	); err == nil || !strings.Contains(err.Error(), "synthesis generation model identity") {
		t.Fatalf("identical live review identity error=%v", err)
	}
	distinct := webIdentityFixture("review", strings.Repeat("b", 64))
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkWebClaimEvidenceReview},
		exactStationExecution{ProviderIdentity: distinct},
	); err != nil {
		t.Fatal(err)
	}
}

func TestWebReviewRejectsDifferentAliasForSameProviderDigest(t *testing.T) {
	guard := &webModelIdentityGuard{}
	digest := strings.Repeat("a", 64)
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkWebGroundedSynthesis},
		exactStationExecution{ProviderIdentity: webIdentityFixture("synthesis-alias", digest)},
	); err != nil {
		t.Fatal(err)
	}
	err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkWebClaimEvidenceReview},
		exactStationExecution{ProviderIdentity: webIdentityFixture("review-alias", digest)},
	)
	if err == nil {
		t.Fatal("two routes resolving to the same provider digest were accepted as independent")
	}
}

func TestWebReReviewIdentityMustDifferFromCorrectionGenerator(t *testing.T) {
	guard := &webModelIdentityGuard{}
	initial := webIdentityFixture("synthesis", strings.Repeat("a", 64))
	correction := webIdentityFixture("correction", strings.Repeat("c", 64))
	if err := guard.validate(assemblyline.PortableJob{Kind: assemblyline.WorkWebGroundedSynthesis}, exactStationExecution{ProviderIdentity: initial}); err != nil {
		t.Fatal(err)
	}
	if err := guard.validate(assemblyline.PortableJob{Kind: assemblyline.WorkWebGroundedSynthesisCorrection}, exactStationExecution{ProviderIdentity: correction}); err != nil {
		t.Fatal(err)
	}
	err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkWebClaimEvidenceReview},
		exactStationExecution{ProviderIdentity: correction},
	)
	if err == nil || !strings.Contains(err.Error(), "synthesis generation model identity") {
		t.Fatalf("correction self-review identity error=%v", err)
	}
}

func TestWebReviewIdentityCannotRunBeforeSynthesisProof(t *testing.T) {
	guard := &webModelIdentityGuard{}
	err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkWebClaimEvidenceReview},
		exactStationExecution{ProviderIdentity: webIdentityFixture("review", strings.Repeat("b", 64))},
	)
	if err == nil || !strings.Contains(err.Error(), "before grounded synthesis identity") {
		t.Fatalf("review-before-synthesis error=%v", err)
	}
}

func webIdentityFixture(model, digest string) llm.ProviderIdentityExpectation {
	return llm.ProviderIdentityExpectation{
		Backend: "ollama", BackendVersion: "0.12.0", Model: model,
		Digest: digest, Quantization: "Q4_K_M", NativeContextLimit: 32768,
		TokenizerProfile: "qwen3",
	}
}
