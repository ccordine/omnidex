package worker

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/ollama"
)

const liveApplicationReviewQualificationModelEnv = "OMNIDEX_TEST_APPLICATION_REVIEW_MODEL"

type liveApplicationReviewQualificationCase struct {
	name          string
	product       string
	requirement   string
	specification assemblyline.ApplicationJobSpecification
	want          assemblyline.ApplicationJobSpecificationReviewDecision
}

func TestLiveApplicationJobSpecificationReviewAuthorityQualification(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveApplicationReviewQualificationModelEnv))
	if modelName == "" {
		t.Skip(liveApplicationReviewQualificationModelEnv + " is not set")
	}
	baseURL := requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_URL")
	contextTokens, err := strconv.Atoi(requireLiveCodingQualificationEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx := context.Background()
	client := ollama.NewUnbounded(baseURL, "", "", contextTokens)
	transport, err := newLiveCodingQualificationTransport(ctx, client, modelName, contextTokens)
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range liveApplicationReviewQualificationCases() {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			requirement := assemblyline.Requirement{
				ID: "requirement_001", SourceQuote: testCase.requirement,
			}
			authority := assemblyline.ApplicationJobSpecificationInput{
				Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: testCase.product,
				AcceptedRequirements: []assemblyline.Requirement{requirement},
				FocusedRequirement:   requirement,
			}
			input, err := assemblyline.NewApplicationJobSpecificationReviewInput(
				authority, testCase.specification, 1,
			)
			if err != nil {
				t.Fatal(err)
			}
			job, err := assemblyline.NewApplicationJobSpecificationReviewJob(input)
			if err != nil {
				t.Fatal(err)
			}
			start := transport.callCount()
			result, err := transport.execute(ctx, job, modelName)
			if err != nil {
				t.Fatal(err)
			}
			if err := result.ValidateFor(job); err != nil {
				t.Fatal(err)
			}
			review, err := assemblyline.DecodeApplicationJobSpecificationReviewResult(
				job, result.Candidate,
			)
			if err != nil {
				t.Fatal(err)
			}
			call := transport.callsFrom(start)[0]
			t.Logf(
				"application_review_qualification model=%s case=%s decision=%s field=%s want=%s wall_ms=%d prompt_tokens=%d output_tokens=%d response_sha256=%s",
				modelName, testCase.name, review.Decision, review.Field, testCase.want,
				call.wallDuration.Milliseconds(), call.promptTokens, call.outputTokens,
				call.responseSHA256,
			)
			if review.Decision != testCase.want {
				t.Errorf("decision=%s field=%s want=%s", review.Decision, review.Field, testCase.want)
			}
		})
	}
}

func liveApplicationReviewQualificationCases() []liveApplicationReviewQualificationCase {
	return []liveApplicationReviewQualificationCase{
		{
			name: "related-derived-choice", product: "browser music studio", requirement: "channels",
			specification: applicationReviewQualificationSpecification(
				"Implement manageable channels in the browser music studio.",
				"Users can rename a channel and see its updated label.",
				"Renaming a channel visibly updates its label.",
			), want: assemblyline.ApplicationJobSpecificationReviewAccept,
		},
		{
			name: "unjustified-hard-constraint", product: "browser music studio", requirement: "channels",
			specification: applicationReviewQualificationSpecification(
				"Implement channels in the browser music studio.",
				"Users can interact with the available channels.",
				"The studio displays exactly eight channels.",
			), want: assemblyline.ApplicationJobSpecificationReviewRepair,
		},
		{
			name: "unrelated-product-scope", product: "browser music studio", requirement: "channels",
			specification: applicationReviewQualificationSpecification(
				"Implement channels and music distribution in the browser music studio.",
				"Users can publish a completed project to a streaming service.",
				"Publishing uploads the project to the selected streaming service.",
			), want: assemblyline.ApplicationJobSpecificationReviewRepair,
		},
		{
			name: "material-persistence-expansion", product: "browser music studio", requirement: "channels",
			specification: applicationReviewQualificationSpecification(
				"Implement persistent channels in the browser music studio.",
				"Channel settings persist across browser sessions.",
				"Reopening the browser restores every prior channel setting.",
			), want: assemblyline.ApplicationJobSpecificationReviewRepair,
		},
		{
			name: "contradicts-product-intent", product: "browser journal", requirement: "works without user accounts",
			specification: applicationReviewQualificationSpecification(
				"Implement account-gated access to the browser journal.",
				"Users must sign in before viewing the journal.",
				"An unauthenticated user cannot view the journal.",
			), want: assemblyline.ApplicationJobSpecificationReviewRepair,
		},
		{
			name: "local-implementation-choice", product: "browser music studio", requirement: "channels",
			specification: applicationReviewQualificationSpecification(
				"Implement channel controls backed by an in-memory ordered collection.",
				"Users can select a channel and adjust its controls.",
				"Selecting a channel exposes its controls, and adjusting one visibly updates that channel.",
			), want: assemblyline.ApplicationJobSpecificationReviewAccept,
		},
	}
}

func applicationReviewQualificationSpecification(
	objective string,
	behavior string,
	criterion string,
) assemblyline.ApplicationJobSpecification {
	return assemblyline.ApplicationJobSpecification{
		Objective: objective, RequiredBehaviors: []string{behavior},
		AcceptanceCriteria: []string{criterion},
	}
}
