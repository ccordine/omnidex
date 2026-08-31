package worker

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

const liveRequirementResultRelationModelEnv = "OMNIDEX_TEST_CODING_REQUIREMENTS_MODEL"

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
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	t.Cleanup(cancel)
	client := ollama.New(baseURL, modelName, "", 5*time.Minute)

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
			name:      "unnamed suitability policy",
			candidate: "The finished software selects the most suitable destination for supplied material.",
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
					return executeLiveRequirementResultRelationJob(
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

func executeLiveRequirementResultRelationJob(
	ctx context.Context,
	client *ollama.Client,
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
	maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(job, contextTokens)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	prepared, err := prepareExactStationCall(exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Prompt: prompt,
		ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens,
	}, modelName, nil)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	generation, err := client.GeneratePreparedExact(ctx, prepared)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(prepared, generation); err != nil {
		return assemblyline.PortableResult{}, err
	}
	projection, err := assemblyline.NewExactPortableResultProjection(generation.Content)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	t.Logf("kind=%s response=%q", job.Kind, generation.Content)
	return assemblyline.PortableResult{
		JobID: job.ID, Candidate: generation.Content, Projection: &projection,
	}, nil
}
