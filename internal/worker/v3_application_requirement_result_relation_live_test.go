package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

const liveRequirementResultRelationModelEnv = "OMNIDEX_TEST_CODING_REQUIREMENT_RESULT_RELATION_MODEL"

func TestLiveRequirementResultRelationOperationFamilyQualification(t *testing.T) {
	modelName := strings.TrimSpace(os.Getenv(liveRequirementResultRelationModelEnv))
	if modelName == "" {
		t.Skip(liveRequirementResultRelationModelEnv + " is not set")
	}
	baseURL := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	if baseURL == "" {
		t.Fatal("OMNIDEX_TEST_OLLAMA_URL is required")
	}
	contextTokens, err := strconv.Atoi(strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_CONTEXT")))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	ctx := t.Context()
	client := ollama.New(baseURL, modelName, "", llm.MaximumModelRequestDuration)

	fixtures := []struct {
		name      string
		candidate string
		want      string
	}{
		{
			name:      "measurement operation family",
			candidate: "The finished software performs unit-conversion operations on supplied measurements.",
			want:      assemblyline.ApplicationRequirementExplicitResultRelation,
		},
		{
			name:      "observation operation family",
			candidate: "The finished software performs statistical aggregation operations on supplied observations.",
			want:      assemblyline.ApplicationRequirementExplicitResultRelation,
		},
		{
			name:      "spatial property measurement",
			candidate: "The finished software reports the dimensions of each transformed image.",
			want:      assemblyline.ApplicationRequirementExplicitResultRelation,
		},
		{
			name:      "collection property measurement",
			candidate: "The finished software reports the item count of each supplied batch.",
			want:      assemblyline.ApplicationRequirementExplicitResultRelation,
		},
		{
			name:      "unnamed suitability policy",
			candidate: "The finished software selects the most suitable destination for supplied material.",
			want:      assemblyline.ApplicationRequirementMissingResultRelation,
		},
		{
			name:      "unnamed qualitative property",
			candidate: "The finished software reports the quality of each transformed image.",
			want:      assemblyline.ApplicationRequirementMissingResultRelation,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			authority := directCodingResultRelationAuthorityFixture(t, fixture.candidate)
			var calls []assemblyline.WorkKind
			runtime := typedWorkerRuntime{
				Context: ctx, MaxAttempts: 1,
				Execute: func(job assemblyline.PortableJob, requestedModel string) (assemblyline.PortableResult, error) {
					calls = append(calls, job.Kind)
					return executeLiveRequirementsSemanticJob(
						ctx, client, contextTokens, requestedModel, job, t,
					)
				},
			}
			result, err := classifyDirectCodingApplicationRequirementCandidateResultRelation(
				runtime, modelName, fixture.candidate,
				authority.Kind, authority.Cardinality, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Relation != fixture.want {
				t.Fatalf("relation=%q, want %q", result.Relation, fixture.want)
			}
			if len(calls) != 2 {
				t.Fatalf("calls=%v, want exactly two bound presence calls", calls)
			}
			t.Logf("model=%s candidate=%q relation=%s calls=%v", modelName, fixture.candidate, result.Relation, calls)
		})
	}
}

func executeLiveRequirementsSemanticJob(
	ctx context.Context,
	client llm.ExactStationClient,
	contextTokens int,
	modelName string,
	job assemblyline.PortableJob,
	t *testing.T,
) (assemblyline.PortableResult, error) {
	t.Helper()
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	return executeLivePortablePrompt(
		ctx, client, contextTokens, modelName, job, prompt, t,
	)
}

func executeLivePortablePrompt(
	ctx context.Context,
	client llm.ExactStationClient,
	contextTokens int,
	modelName string,
	job assemblyline.PortableJob,
	prompt string,
	t *testing.T,
) (assemblyline.PortableResult, error) {
	t.Helper()
	maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(job, contextTokens)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	prepared, err := prepareExactStationCall(exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Iteration: 1, Prompt: prompt,
		ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens,
	}, modelName, nil)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	generation, err := generatePreparedExactWithinMaximumDuration(ctx, client, prepared)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	validationErr := llm.ValidateExactPreparedGenerationForRequest(prepared, generation)
	var outputLimit *llm.ExactPreparedOutputLimitReachedError
	if errors.As(validationErr, &outputLimit) {
		nextMaximum := prepared.ContextTokens - outputLimit.PromptTokens
		if nextMaximum <= prepared.MaxOutputTokens || nextMaximum >= prepared.ContextTokens {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"live exact station reached its hard native-context authority: %w",
				validationErr,
			)
		}
		continued := prepared
		continued.MaxOutputTokens = nextMaximum
		if _, err := llm.ExactPreparedRequestBytes(continued); err != nil {
			return assemblyline.PortableResult{}, err
		}
		generation, err = generatePreparedExactWithinMaximumDuration(ctx, client, continued)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		prepared = continued
		validationErr = llm.ValidateExactPreparedGenerationForRequest(prepared, generation)
	}
	t.Logf("kind=%s response=%q done_reason=%s output_tokens=%d", job.Kind,
		generation.Content, generation.ProviderDoneReason, generation.Usage.EvalCount)
	if validationErr != nil {
		return assemblyline.PortableResult{}, validationErr
	}
	projection, err := assemblyline.NewExactPortableResultProjection(generation.Content)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	return assemblyline.PortableResult{
		JobID: job.ID, Candidate: generation.Content, Projection: &projection,
	}, nil
}
