package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/repository/cognitionenv"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestRepositoryCognitionNeedUsesOnlyAcceptedBoundedQuery(t *testing.T) {
	t.Parallel()
	decision := assemblyline.RepositoryRetrievalDecision{
		Schema:    assemblyline.RepositoryRetrievalSchemaV2,
		Operation: assemblyline.RetrievalDirectReferences, QueryQuote: "Value",
	}
	need, err := repositoryCognitionNeedAuthority(decision)
	if err != nil || need.Content != decision.QueryQuote ||
		strings.Contains(need.Content, strings.Repeat("unrelated", 600)) {
		t.Fatalf("repository cognition need=%+v error=%v", need, err)
	}
	decision.QueryQuote = "private/value.go"
	if _, err := repositoryCognitionNeedAuthority(decision); err == nil {
		t.Fatal("path-bearing accepted query entered repository cognition")
	}
}

func TestRepositoryCognitionBudgetIsBoundToOperationCatalog(t *testing.T) {
	t.Parallel()
	brain := repositoryCognitionTestBrain(t, "analyze-model")
	for _, testCase := range []struct {
		operation repositoryretrieval.Operation
		decisions int
		arguments int
	}{
		{repositoryretrieval.OperationSemanticExcerpts, 1, 0},
		{repositoryretrieval.OperationSymbolDeclaration, 2, 1},
		{repositoryretrieval.OperationDirectReferences, 3, 1},
	} {
		catalog, err := cognitionenv.RegisteredCatalog(testCase.operation)
		if err != nil {
			t.Fatal(err)
		}
		budget, cycles, err := repositoryCognitionBudget(brain, catalog)
		if err != nil {
			t.Fatal(err)
		}
		if budget.MaxInputBytes != brain.ContextCeilingBytes ||
			budget.MaxOutputTokens != brain.Sampling.MaxOutputTokens ||
			budget.RemainingPolicyCalls != uint32(testCase.decisions) || cycles != testCase.decisions+1 ||
			budget.MaxEvidenceRefs != 1+(2*testCase.decisions) ||
			budget.MaxActionArguments != testCase.arguments || budget.MaxLedgerProposals != 0 ||
			budget.MaxAttentionRequests != 0 {
			t.Fatalf("operation=%s budget=%+v cycles=%d", testCase.operation, budget, cycles)
		}
	}
	changed := brain
	changed.ContextCeilingBytes = 1
	catalog, _ := cognitionenv.RegisteredCatalog(repositoryretrieval.OperationSemanticExcerpts)
	if _, _, err := repositoryCognitionBudget(changed, catalog); err == nil {
		t.Fatal("divergent brain identity produced a runtime budget")
	}
}

func TestRepositoryCognitionRequiresExactJobLocalAnalyzeRoute(t *testing.T) {
	t.Parallel()
	brain := repositoryCognitionTestBrain(t, "analyze-model")
	runtime := &nativeRuntimeV3{
		svc:     &Service{cognitionBrain: brain},
		claim:   &model.ClaimedStep{Job: model.Job{Metadata: []byte(`{"model_analyze":"other-model"}`)}},
		routing: ModelRouting{Analyze: "analyze-model"},
	}
	if _, err := repositoryCognitionBrain(runtime); err == nil ||
		!strings.Contains(err.Error(), "differs from job-local analyze route") {
		t.Fatalf("route mismatch error=%v", err)
	}
	runtime.claim.Job.Metadata = []byte(`{"model_analyze":"analyze-model"}`)
	if got, err := repositoryCognitionBrain(runtime); err != nil || got != brain {
		t.Fatalf("resolved brain=%+v error=%v", got, err)
	}
}

func TestRepositoryCognitionEpisodeIdentitySurvivesAttemptReplacement(t *testing.T) {
	t.Parallel()
	scenario, err := cognition.NewScenarioRef("repository-scenario", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	authority := model.StepAttemptAuthority{
		JobID: 7, Generation: 3, StepID: 11, Attempt: 1, WorkerID: "worker-one",
	}
	first, err := repositoryCognitionEpisodeRef(authority, scenario)
	if err != nil {
		t.Fatal(err)
	}
	authority.Attempt, authority.WorkerID = 2, "worker-two"
	replacement, err := repositoryCognitionEpisodeRef(authority, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if first != replacement {
		t.Fatalf("replacement attempt changed episode identity: %q != %q", first.ID, replacement.ID)
	}
	authority.StepID++
	changed, err := repositoryCognitionEpisodeRef(authority, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("another step reused a repository cognition episode")
	}
}

func repositoryCognitionTestBrain(t *testing.T, modelName string) cognitionpolicy.BrainRef {
	t.Helper()
	sampling, err := cognitionpolicy.NewSamplingIdentity(32768, 24576, 4096)
	if err != nil {
		t.Fatal(err)
	}
	brain, err := cognitionpolicy.NewBrainRef(
		modelName, strings.Repeat("a", 64), "Q4_K_M", "ollama", "0.24.0", "test-hardware", sampling,
	)
	if err != nil {
		t.Fatal(err)
	}
	return brain
}
