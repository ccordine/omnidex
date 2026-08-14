package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
	"github.com/gryph/omnidex/internal/ollama"
)

func TestLiveObjectiveAdvisoryExactTransportAndReduction(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	if baseURL == "" {
		t.Skip("OMNIDEX_TEST_OLLAMA_URL is not set")
	}
	model := requireLiveAdvisoryEnv(t, "OMNIDEX_TEST_OLLAMA_MODEL")
	embeddingModel := requireLiveAdvisoryEnv(t, "OMNIDEX_TEST_OLLAMA_EMBEDDING_MODEL")
	contextTokens, err := strconv.Atoi(requireLiveAdvisoryEnv(t, "OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if err != nil || contextTokens <= 0 {
		t.Fatal("OMNIDEX_TEST_OLLAMA_CONTEXT must be a positive integer")
	}
	client := ollama.New(baseURL, model, embeddingModel, 5*time.Minute, contextTokens)
	for _, requiredModel := range []string{model, embeddingModel} {
		available, err := client.HasModel(t.Context(), requiredModel)
		if err != nil {
			t.Fatalf("inspect live Ollama model %q: %v", requiredModel, err)
		}
		if !available {
			t.Fatalf("required live Ollama model %q is not installed", requiredModel)
		}
	}

	cases := []liveAdvisoryCase{
		{
			name: "typed-adapter", objective: "Review a candidate explanation of a typed callback boundary.",
			evidence:  "A named adapter implements the required method by invoking its underlying function; the bare function type has no methods.",
			candidate: "Any function with matching parameters already implements the interface and needs no adapter.",
		},
		{
			name: "exclusive-resource", objective: "Review a candidate earliest-finish calculation with dependency and exclusivity constraints.",
			evidence:  "After a three-unit prerequisite, two tasks of four and two units share one exclusive resource; a final one-unit task waits for both.",
			candidate: "The two middle tasks overlap, so the earliest finish is eight units.",
		},
	}
	for _, testCase := range cases {
		if passed := t.Run(testCase.name, func(t *testing.T) {
			input, gap := liveAdvisoryInputs(testCase)
			off, err := objectiveadvisory.New(objectiveadvisory.Config{Mode: objectiveadvisory.ModeOff}, nil, nil, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			offReport, err := off.Run(t.Context(), input, gap)
			if err != nil {
				t.Fatal(err)
			}
			if offReport.Metrics.AdvisoryCalls != 0 || len(offReport.Artifacts) != 0 ||
				len(offReport.ActiveCapsules) != 0 || offReport.Metrics.SelectedCapsuleContentBytes != 0 {
				t.Fatalf("off mode exposed advisory work: %#v", offReport.Metrics)
			}

			recorder := &liveExactAdvisoryClient{ExactStationClient: client}
			runtime, err := objectiveadvisory.New(liveAdvisoryConfig(model),
				exactObjectiveAdvisoryProvider{client: recorder, contextTokens: contextTokens}, client, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			report, err := runtime.Run(ctx, input, gap)
			if err != nil {
				t.Fatal(err)
			}
			logLiveAdvisoryReport(t, testCase.name, report, recorder)
			assertLiveAdvisoryReport(t, offReport, report, recorder)
		}); !passed {
			return
		}
	}
}

type liveAdvisoryCase struct{ name, objective, evidence, candidate string }

type liveExactCall struct{ requestSHA256, responseSHA256 string }

type liveExactAdvisoryClient struct {
	llm.ExactStationClient
	calls []liveExactCall
}

func (client *liveExactAdvisoryClient) GeneratePreparedExact(
	ctx context.Context, prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	requestSHA256, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	result, callErr := client.ExactStationClient.GeneratePreparedExact(ctx, prepared)
	client.calls = append(client.calls, liveExactCall{
		requestSHA256: requestSHA256, responseSHA256: result.ProviderResponseSHA256,
	})
	return result, callErr
}

func liveAdvisoryConfig(model string) objectiveadvisory.Config {
	return objectiveadvisory.Config{
		Mode: objectiveadvisory.ModeActive, MinimumRelevance: objectiveAdvisoryMinimumRelevance,
		MaxSelectedCapsules: 1, Sources: []objectiveadvisory.SourceConfig{{
			ID: objectiveAdvisorySourceID, Provider: llm.ExactPreparedProviderBackend, Model: model,
			Sampling: objectiveadvisory.SamplingConfig{Temperature: 0},
			Budget: objectiveadvisory.Budget{MaxInputBytes: objectiveAdvisoryMaxInputBytes,
				MaxOutputBytes: objectiveAdvisoryMaxOutputBytes, MaxOutputTokens: objectiveAdvisoryMaxOutputTokens},
		}},
	}
}

func liveAdvisoryInputs(testCase liveAdvisoryCase) (objectiveadvisory.ProjectionInput, objectiveadvisory.SemanticGap) {
	digest := sha256.Sum256([]byte(testCase.evidence))
	evidence := []objectiveadvisory.EvidenceSummary{{
		ID: "E01", Summary: testCase.evidence, SHA256: hex.EncodeToString(digest[:]),
	}}
	input := objectiveadvisory.ProjectionInput{
		ObjectiveID: "live-" + testCase.name, Generation: 1, Objective: testCase.objective,
		UserAuthorities: []objectiveadvisory.TextAuthority{{ID: "U01", Content: testCase.objective}},
		Constraints:     []string{}, GroundedEvidence: evidence, Decisions: []string{},
		Invariants: []string{}, UnresolvedQuestions: []string{},
		UsefulAdvice: "Identify bounded risks, hidden constraints, or verification questions relevant to reviewing the candidate.",
	}
	return input, objectiveadvisory.SemanticGap{ObjectiveID: input.ObjectiveID, Generation: 1,
		Requirement: testCase.objective, Candidate: testCase.candidate, Evidence: evidence}
}

func assertLiveAdvisoryReport(t *testing.T, off, report objectiveadvisory.Report, client *liveExactAdvisoryClient) {
	t.Helper()
	if report.Projection.ID != off.Projection.ID || report.Metrics.AdvisoryCalls != 1 ||
		len(report.Artifacts) != 1 || report.Artifacts[0].Status != objectiveadvisory.StatusSucceeded ||
		report.Metrics.RawBytes == 0 || report.Metrics.ChunksProduced == 0 ||
		report.Metrics.CandidateCapsules == 0 || report.Metrics.SelectedCapsules != 1 ||
		len(report.ActiveCapsules) != 1 || len(client.calls) != 1 {
		t.Fatalf("live advisory did not complete the bounded vertical: report=%#v calls=%d", report.Metrics, len(client.calls))
	}
	artifact, capsule := report.Artifacts[0], report.ActiveCapsules[0]
	if artifact.Authority != objectiveadvisory.AuthorityNonAuthoritative ||
		capsule.Authority != objectiveadvisory.AuthorityNonAuthoritative ||
		capsule.Label != objectiveadvisory.CapsuleLabel ||
		capsule.SourceAdvisoryID != artifact.ID || client.calls[0].requestSHA256 == "" ||
		client.calls[0].responseSHA256 == "" {
		t.Fatal("live advisory lost exact provenance or non-authoritative scope")
	}
}

func logLiveAdvisoryReport(
	t *testing.T, caseName string, report objectiveadvisory.Report, client *liveExactAdvisoryClient,
) {
	t.Helper()
	artifact := objectiveadvisory.Artifact{}
	if len(report.Artifacts) == 1 {
		artifact = report.Artifacts[0]
	}
	call := liveExactCall{}
	if len(client.calls) == 1 {
		call = client.calls[0]
	}
	relevance := ""
	if len(report.ActiveCapsules) == 1 {
		relevance = report.ActiveCapsules[0].RelevanceBasis
	}
	t.Logf("live_objective_advisory case=%s status=%s provider=%s model=%s digest=%s quantization=%s projection_sha256=%s request_sha256=%s response_sha256=%s raw_sha256=%s raw_bytes=%d chunks=%d candidates=%d selected=%d unused=%d potential_bytes=%d downstream_bytes=%d downstream_tokens=%d prompt_tokens=%d output_tokens=%d provider_ms=%d wall_ms=%d relevance=%s failure=%q",
		caseName, artifact.Status, artifact.EffectiveProvider, artifact.EffectiveModel,
		artifact.ModelDigest, artifact.Quantization, report.Projection.RenderedSHA256,
		call.requestSHA256, call.responseSHA256, artifact.RawTextSHA256,
		report.Metrics.RawBytes, report.Metrics.ChunksProduced,
		report.Metrics.CandidateCapsules, report.Metrics.SelectedCapsules,
		report.Metrics.UnselectedChunks, report.Metrics.PotentialCapsuleContentBytes,
		report.Metrics.SelectedCapsuleContentBytes, report.Metrics.SelectedCapsuleContentTokens,
		report.Metrics.PromptTokens, report.Metrics.OutputTokens,
		artifact.Duration.Milliseconds(), report.Metrics.WallTime.Milliseconds(),
		relevance, artifact.Failure)
}

func requireLiveAdvisoryEnv(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s must be set when live objective advisory evaluation is enabled", key)
	}
	return value
}
