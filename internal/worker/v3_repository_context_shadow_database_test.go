package worker

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repositoryShadowTestLLM struct {
	prompts []string
}

func (*repositoryShadowTestLLM) Generate(context.Context, string, string) (string, error) {
	return "", nil
}

func (*repositoryShadowTestLLM) PrepareContextModel(_ context.Context, modelName, prompt string) (llm.PreparedModel, error) {
	return llm.PreparedModel{BaseModel: modelName, ContextModel: modelName, Prompt: prompt}, nil
}

func (client *repositoryShadowTestLLM) GeneratePrepared(_ context.Context, prepared llm.PreparedModel) (string, error) {
	client.prompts = append(client.prompts, prepared.Prompt)
	return `{"ok":true}`, nil
}

func (*repositoryShadowTestLLM) CleanupPreparedModel(llm.PreparedModel) {}

func (*repositoryShadowTestLLM) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func TestPostgresRepositoryShadowConsumerPreservesPromptAndBindsEvidence(t *testing.T) {
	ctx, repository, _ := openRepositoryShadowDatabase(t)
	claim := startRepositoryShadowJob(t, ctx, repository, "shadow-live")
	client := &repositoryShadowTestLLM{}
	service := &Service{
		repo: repository, llm: client, inferenceContextTokens: 8192, fragmentConcurrency: 1,
		logger: log.New(io.Discard, "", 0),
	}
	runtime := &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}
	session := &directCodingSession{runtime: runtime, repositoryIndex: &repositoryindex.Result{}}
	execute := directCodingWorkerRuntime(session).Execute

	retrievalJob, err := assemblyline.NewRepositoryRetrievalJob(
		assemblyline.RepositoryRetrievalInput{ResearchNeed: "Change Owner behavior."},
	)
	if err != nil {
		t.Fatal(err)
	}
	retrievalPrompt, _, err := assemblyline.RenderPortableJob(retrievalJob)
	if err != nil {
		t.Fatal(err)
	}
	for repetition := 0; repetition < 2; repetition++ {
		if _, err := execute(retrievalJob, "qwen-test"); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := repository.CurrentWorkingSet(ctx, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 1 || len(snapshot.Items) != 1 {
		t.Fatalf("retrieval resume mutated its exact working set: %+v", snapshot)
	}

	pack := repositoryProjectionTestPack(t)
	changeJob, err := assemblyline.NewRepositoryChangeSurfaceJob(assemblyline.RepositoryChangeSurfaceInput{
		ResearchNeed: "Change Owner behavior.", RequirementQuotes: []string{"Owner behavior"}, Evidence: pack,
	})
	if err != nil {
		t.Fatal(err)
	}
	changePrompt, _, err := assemblyline.RenderPortableJob(changeJob)
	if err != nil {
		t.Fatal(err)
	}
	for repetition := 0; repetition < 2; repetition++ {
		if _, err := execute(changeJob, "qwen-test"); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err = repository.CurrentWorkingSet(ctx, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 2 || len(snapshot.Items) != 2 {
		t.Fatalf("change-surface resume mutated deterministic acquisitions: %+v", snapshot)
	}
	if len(client.prompts) != 4 || client.prompts[0] != retrievalPrompt ||
		client.prompts[1] != retrievalPrompt || client.prompts[2] != changePrompt ||
		client.prompts[3] != changePrompt {
		t.Fatalf("shadow projection changed model-visible prompts: %#v", client.prompts)
	}

	projectionIDs := assertRepositoryShadowProjections(t, ctx, repository, claim.Job.ID, retrievalJob, changeJob)
	assertRepositoryShadowLLMEvidence(t, ctx, repository, claim.Job.ID, []string{
		retrievalPrompt, retrievalPrompt, changePrompt, changePrompt,
	}, []string{
		projectionIDs[0], projectionIDs[0], projectionIDs[1], projectionIDs[1],
	})
}

func TestPostgresRepositoryShadowConsumerRejectsMismatchedExistingSet(t *testing.T) {
	ctx, repository, _ := openRepositoryShadowDatabase(t)
	claim := startRepositoryShadowJob(t, ctx, repository, "shadow-mismatch")
	if _, err := repository.CreateCurrentWorkingSet(
		ctx, claim.Authority,
		workingset.Budget{MaxItems: 1, MaxBytes: 1024},
	); err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewRepositoryRetrievalJob(
		assemblyline.RepositoryRetrievalInput{ResearchNeed: "Find one owner."},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareRepositoryShadowProjection(ctx, repository, claim, job); err == nil ||
		!strings.Contains(err.Error(), "mismatched owner, lifecycle, or fixed budget") {
		t.Fatalf("mismatched working set error=%v", err)
	}
}

func assertRepositoryShadowProjections(
	t *testing.T,
	ctx context.Context,
	repository *queue.Repository,
	jobID int64,
	retrievalJob, changeJob assemblyline.PortableJob,
) [2]string {
	t.Helper()
	page, err := repository.ListContextProjectionSummaries(ctx, jobID, 1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Authority.Mode != queue.ContextProjectionModeShadow ||
		page[1].Authority.Mode != queue.ContextProjectionModeShadow ||
		page[0].WorkID != retrievalJob.ID || page[1].WorkID != changeJob.ID {
		t.Fatalf("projection summaries=%+v", page)
	}
	retrieval, err := repository.GetContextProjection(ctx, page[0].ProjectionID)
	if err != nil {
		t.Fatal(err)
	}
	change, err := repository.GetContextProjection(ctx, page[1].ProjectionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(retrieval.Projection.Selected) != 1 ||
		retrieval.Projection.Selected[0].Role != workingset.RoleUserAuthority ||
		retrieval.Projection.Selected[0].Authority != taskstate.AuthorityUser {
		t.Fatalf("retrieval selection=%+v", retrieval.Projection.Selected)
	}
	if len(change.Projection.Selected) != 2 ||
		change.Projection.Selected[1].Role != workingset.RoleRepositoryEvidence ||
		change.Projection.Selected[1].Authority != taskstate.AuthorityToolEvidence {
		t.Fatalf("change selection=%+v", change.Projection.Selected)
	}
	return [2]string{page[0].ProjectionID, page[1].ProjectionID}
}

func assertRepositoryShadowLLMEvidence(
	t *testing.T,
	ctx context.Context,
	repository *queue.Repository,
	jobID int64,
	wantPrompts []string,
	wantProjectionIDs []string,
) {
	t.Helper()
	page, err := repository.ReadJobHistoryPage(ctx, jobID, queue.JobHistoryRequest{
		Stream: queue.JobHistoryLLMCalls, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.LLMCalls) != len(wantPrompts) || len(wantPrompts) != len(wantProjectionIDs) {
		t.Fatalf("LLM calls=%+v", page.LLMCalls)
	}
	for index, historical := range page.LLMCalls {
		if historical.Call.ContextProjectionID != wantProjectionIDs[index] ||
			historical.Call.SystemPrompt != wantPrompts[index] ||
			historical.Call.JobGeneration != 1 || historical.Step.Generation != 1 {
			t.Fatalf("LLM call %d lost exact shadow binding: %+v", index, historical)
		}
	}
}

func openRepositoryShadowDatabase(t *testing.T) (context.Context, *queue.Repository, *pgxpool.Pool) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL repository shadow tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(ctx, loadWorkerTestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	return ctx, repository, pool
}

func startRepositoryShadowJob(
	t *testing.T,
	ctx context.Context,
	repository *queue.Repository,
	label string,
) *model.ClaimedStep {
	t.Helper()
	job, err := repository.EnqueueJob(
		ctx, label+"-"+time.Now().UTC().Format("150405.000000000"), model.PipelineCoding, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, label+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claimed shadow job=%+v want job %d", claim, job.ID)
	}
	return claim
}
