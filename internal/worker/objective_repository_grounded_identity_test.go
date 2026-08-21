package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRepositoryReviewRoutesMustBeConfiguredAndDistinct(t *testing.T) {
	t.Parallel()
	routing := ModelRouting{Stations: map[station.ID]string{
		station.GroundedAnswer:               "answer-model",
		station.RepositoryGroundedCorrection: "answer-model",
		station.RepositoryGroundedReview:     "answer-model",
	}}
	if err := requireIndependentRepositoryReviewRoutes(routing); err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("equal answer/review route error=%v", err)
	}
	routing.Stations[station.RepositoryGroundedReview] = "review-model"
	if err := requireIndependentRepositoryReviewRoutes(routing); err != nil {
		t.Fatal(err)
	}
	routing.Stations[station.RepositoryGroundedCorrection] = "review-model"
	if err := requireIndependentRepositoryReviewRoutes(routing); err == nil || !strings.Contains(err.Error(), "correction") {
		t.Fatalf("equal correction/review route error=%v", err)
	}
	delete(routing.Stations, station.RepositoryGroundedReview)
	if err := requireIndependentRepositoryReviewRoutes(routing); err == nil || !strings.Contains(err.Error(), "no configured model") {
		t.Fatalf("missing review route error=%v", err)
	}
}

func TestRepositoryReviewIdentityMustDifferFromAnswerAndCorrection(t *testing.T) {
	t.Parallel()
	answer := webIdentityFixture("answer", strings.Repeat("a", 64))
	review := webIdentityFixture("review", strings.Repeat("b", 64))
	correction := webIdentityFixture("correction", strings.Repeat("c", 64))
	guard := &repositoryGroundingModelIdentityGuard{}
	if err := guard.validate(assemblyline.PortableJob{Kind: assemblyline.WorkGroundedAnswer}, exactStationExecution{ProviderIdentity: answer}); err != nil {
		t.Fatal(err)
	}
	if err := guard.validate(assemblyline.PortableJob{Kind: assemblyline.WorkRepositoryGroundedReview}, exactStationExecution{ProviderIdentity: answer}); err == nil {
		t.Fatal("answer model reviewed its own answer")
	}
	if err := guard.validate(assemblyline.PortableJob{Kind: assemblyline.WorkRepositoryGroundedReview}, exactStationExecution{ProviderIdentity: review}); err != nil {
		t.Fatal(err)
	}
	if err := guard.validate(assemblyline.PortableJob{Kind: assemblyline.WorkRepositoryGroundedCorrection}, exactStationExecution{ProviderIdentity: correction}); err != nil {
		t.Fatal(err)
	}
	if err := guard.validate(assemblyline.PortableJob{Kind: assemblyline.WorkRepositoryGroundedReview}, exactStationExecution{ProviderIdentity: correction}); err == nil {
		t.Fatal("correction model reviewed its own correction")
	}
}

func TestRepositoryReviewRejectsAliasForSameExactDigest(t *testing.T) {
	t.Parallel()
	guard := &repositoryGroundingModelIdentityGuard{}
	digest := strings.Repeat("a", 64)
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkGroundedAnswer},
		exactStationExecution{ProviderIdentity: webIdentityFixture("answer-alias", digest)},
	); err != nil {
		t.Fatal(err)
	}
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkRepositoryGroundedReview},
		exactStationExecution{ProviderIdentity: webIdentityFixture("review-alias", digest)},
	); err == nil {
		t.Fatal("different model route names resolved to the same exact provider identity")
	}
}

func TestRepositoryCorrectionCannotAliasItsIndependentReviewer(t *testing.T) {
	t.Parallel()
	guard := &repositoryGroundingModelIdentityGuard{}
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkGroundedAnswer},
		exactStationExecution{ProviderIdentity: webIdentityFixture("answer", strings.Repeat("a", 64))},
	); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("b", 64)
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkRepositoryGroundedReview},
		exactStationExecution{ProviderIdentity: webIdentityFixture("review", digest)},
	); err != nil {
		t.Fatal(err)
	}
	err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkRepositoryGroundedCorrection},
		exactStationExecution{ProviderIdentity: webIdentityFixture("correction-alias", digest)},
	)
	if err == nil || !strings.Contains(err.Error(), "review model identity") {
		t.Fatalf("error=%v", err)
	}
}

func TestRepositoryReviewIdentityCannotRunBeforeGenerationProof(t *testing.T) {
	t.Parallel()
	guard := &repositoryGroundingModelIdentityGuard{}
	err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkRepositoryGroundedReview},
		exactStationExecution{ProviderIdentity: webIdentityFixture("review", strings.Repeat("b", 64))},
	)
	if err == nil || !strings.Contains(err.Error(), "before repository answer generation") {
		t.Fatalf("error=%v", err)
	}
}

func TestRepositoryReviewValidationRetryRetainsIndependentIdentityGuard(t *testing.T) {
	t.Parallel()
	guard := &repositoryGroundingModelIdentityGuard{}
	answerIdentity := webIdentityFixture("answer", strings.Repeat("a", 64))
	if err := guard.validate(
		assemblyline.PortableJob{Kind: assemblyline.WorkGroundedAnswer},
		exactStationExecution{ProviderIdentity: answerIdentity},
	); err != nil {
		t.Fatal(err)
	}
	reviewInput := assemblyline.RepositoryGroundedReviewInput{
		RequirementID: "requirement-1", ExactRequirement: "Which component owns dispatch?",
		AnswerText: "DispatchOwner owns dispatch.", EvidenceIDs: []string{"R01"},
		Evidence: []assemblyline.GroundedEvidenceCapsule{{ID: "R01", Text: "DispatchOwner owns dispatch."}},
	}
	reviewJob, err := assemblyline.NewRepositoryGroundedReviewJob(reviewInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assemblyline.NewRetainedResponseCorrectionJob(
		reviewJob, "outcome is invalid",
		`{"schema":"omnidex.repository-grounded-review.v1","outcome":"invalid","issue_kind":"","detail":""}`,
	); err == nil || !strings.Contains(err.Error(), "exactly one code-owned mutable semantic field") {
		t.Fatalf("multi-leaf repository review correction was accepted: %v", err)
	}
}
