package worker

import (
	"io"
	"log"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/repository/cognitionenv"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestPostgresRepositoryCognitionShadowDrivesMultiStepReadOnlyInvestigation(t *testing.T) {
	ctx, repository, pool := openRepositoryCognitionDatabase(t)
	root := repositoryMutationWorkflowRoot(t)
	project, err := repository.CreateProject(ctx, "repository-cognition-integration", root, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	indexer, err := repositoryindex.New(repository)
	if err != nil {
		t.Fatal(err)
	}
	indexed, err := indexer.Refresh(ctx, project.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	claim := claimReplannedRepositoryCognitionJob(t, ctx, repository, project.ID, root)
	unrelatedCanary := "UNRELATED_CONVERSATION_AUTHORITY_MUST_NOT_ENTER_COGNITION"
	claim.Job.Instruction = unrelatedCanary + strings.Repeat(" irrelevant", 600)
	if len(claim.Job.Instruction) <= 4*1024 {
		t.Fatal("unrelated authority fixture must exceed the cognition need limit")
	}
	if _, err := repository.CreateCurrentWorkingSet(
		ctx, claim.Authority, repositoryShadowWorkingSetBudget(),
	); err != nil {
		t.Fatal(err)
	}
	retrieval, err := repositoryretrieval.New(repository)
	if err != nil {
		t.Fatal(err)
	}
	brain := repositoryCognitionTestBrain(t, "repository-cognition-test")
	client := &repositoryCognitionTestClient{
		model: brain.Model, native: brain.NativeContextLimit,
		output: brain.Sampling.MaxOutputTokens,
	}
	service := &Service{
		repo: repository, llm: client, cognitionBrain: brain,
		repositoryRetrieval: retrieval, logger: log.New(io.Discard, "", 0),
	}
	runtime := &nativeRuntimeV3{
		svc: service, ctx: ctx, claim: claim,
		routing: ModelRouting{Analyze: brain.Model}, contexts: map[string]string{},
	}
	session := &directCodingSession{
		runtime: runtime, root: root, repositoryIndex: &indexed,
	}
	decision := assemblyline.RepositoryRetrievalDecision{
		Schema:    assemblyline.RepositoryRetrievalSchemaV2,
		Operation: assemblyline.RetrievalDirectReferences, QueryQuote: "Value",
	}
	if err := session.runRepositoryCognitionShadow(decision, indexed.Analyses[0].ID); err != nil {
		t.Fatal(err)
	}

	var episodeID, status string
	var actions int
	if err := pool.QueryRow(ctx, `
		SELECT episode_id,status,action_count FROM cognition_episodes WHERE job_id=$1
	`, claim.Job.ID).Scan(&episodeID, &status, &actions); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || actions != 3 {
		var cancellation, policyResult, projection string
		_ = pool.QueryRow(ctx, `
			SELECT COALESCE((SELECT source_evidence_json FROM cognition_episode_cancellations
			                 WHERE episode_id=$1),''),
			       COALESCE((SELECT result_json FROM cognition_policy_calls
			                 WHERE episode_id=$1 ORDER BY created_at DESC LIMIT 1),''),
			       COALESCE((SELECT projections.rendered_context
			                 FROM cognition_policy_calls calls
			                 JOIN context_projections projections USING (projection_id)
			                 WHERE calls.episode_id=$1 ORDER BY calls.created_at DESC LIMIT 1),'')
		`, episodeID).Scan(&cancellation, &policyResult, &projection)
		t.Fatalf("repository cognition episode=%s status=%s actions=%d cancellation=%s policy=%s projection=%s",
			episodeID, status, actions, cancellation, policyResult, projection)
	}
	var episodeJobGeneration, graphPlanGeneration, rootPlanGeneration, rootJobGeneration int64
	if err := pool.QueryRow(ctx, `
		SELECT episodes.generation,
		       (graphs.graph_json::jsonb->>'generation')::bigint,
		       obligations.created_generation,obligations.job_generation
		FROM cognition_episodes episodes
		JOIN LATERAL (
			SELECT graph_json FROM cognition_obligation_graphs
			WHERE episode_id=episodes.episode_id ORDER BY graph_version DESC LIMIT 1
		) graphs ON TRUE
		JOIN cognition_obligations obligations
		  ON obligations.episode_id=episodes.episode_id
		 AND obligations.node_id=graphs.graph_json::jsonb->>'root_id'
		WHERE episodes.episode_id=$1
	`, episodeID).Scan(
		&episodeJobGeneration, &graphPlanGeneration, &rootPlanGeneration, &rootJobGeneration,
	); err != nil {
		t.Fatal(err)
	}
	if episodeJobGeneration != 2 || rootJobGeneration != 2 ||
		graphPlanGeneration != int64(cognition.InitialObligationGeneration) ||
		rootPlanGeneration != int64(cognition.InitialObligationGeneration) {
		t.Fatalf("repository cognition job/plan/root/root-job generation=%d/%d/%d/%d",
			episodeJobGeneration, graphPlanGeneration, rootPlanGeneration, rootJobGeneration)
	}
	var projections, relevantProjections, unrelatedProjections int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE STRPOS(projections.rendered_context,$2)>0),
		       COUNT(*) FILTER (WHERE STRPOS(projections.rendered_context,$3)>0)
		FROM cognition_policy_calls calls
		JOIN context_projections projections USING (projection_id)
		WHERE calls.episode_id=$1
	`, episodeID, decision.QueryQuote, unrelatedCanary).Scan(
		&projections, &relevantProjections, &unrelatedProjections,
	); err != nil {
		t.Fatal(err)
	}
	if projections != 3 || relevantProjections != projections || unrelatedProjections != 0 {
		t.Fatalf("repository cognition projection relevant/unrelated=%d/%d of %d",
			relevantProjections, unrelatedProjections, projections)
	}
	rows, err := pool.Query(ctx, `
		SELECT registered_action_json::jsonb#>>'{request,kind}',
		       COALESCE(registered_action_json::jsonb#>>'{request,arguments,0,value}','')
		FROM cognition_actions WHERE episode_id=$1 ORDER BY created_at,action_id
	`, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	kinds := make([]string, 0, 3)
	subjects := make([]string, 0, 3)
	for rows.Next() {
		var kind, subject string
		if err := rows.Scan(&kind, &subject); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, kind)
		subjects = append(subjects, subject)
	}
	wantKinds := []string{
		string(cognitionenv.ActionSearch), string(cognitionenv.ActionInspect), string(cognitionenv.ActionReferences),
	}
	if strings.Join(kinds, ",") != strings.Join(wantKinds, ",") {
		t.Fatalf("repository cognition actions=%v want=%v", kinds, wantKinds)
	}
	targetID := ""
	for _, symbol := range indexed.Analyses[0].Symbols {
		if symbol.Name == "Value" {
			targetID = symbol.ID
			break
		}
	}
	if targetID == "" || len(subjects) != 3 || subjects[0] != "" ||
		subjects[1] != targetID || subjects[2] != targetID {
		t.Fatalf("repository cognition subjects=%v target=%q", subjects, targetID)
	}
	var seals, mutations int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM repository_mutation_operations WHERE job_id=$2)
	`, episodeID, claim.Job.ID).Scan(&seals, &mutations); err != nil {
		t.Fatal(err)
	}
	if seals != 1 || mutations != 0 || client.prepared != 3 || client.generated != 3 ||
		client.cleaned != 3 || client.attestations != 5 || client.plain != 0 {
		t.Fatalf("seal/mutation/prepared/generated/cleanup/attest/plain=%d/%d/%d/%d/%d/%d/%d",
			seals, mutations, client.prepared, client.generated, client.cleaned,
			client.attestations, client.plain)
	}
	for _, prompt := range client.prompts {
		hasNeed := strings.Contains(prompt, decision.QueryQuote)
		hasRoot := strings.Contains(prompt, indexed.Snapshot.Root)
		hasUnrelated := strings.Contains(prompt, unrelatedCanary)
		if !hasNeed || hasRoot || hasUnrelated || len(prompt) > brain.ContextCeilingBytes {
			t.Fatalf("repository cognition prompt need/root/unrelated/bytes/ceiling=%t/%t/%t/%d/%d",
				hasNeed, hasRoot, hasUnrelated, len(prompt), brain.ContextCeilingBytes)
		}
		for _, file := range indexed.Snapshot.Files {
			if strings.Contains(prompt, file.Path) || strings.Contains(prompt, file.ID) {
				t.Fatalf("repository cognition prompt exposed file identity")
			}
		}
	}
	before := loadRepositoryCognitionCounts(t, ctx, pool, episodeID, claim.Job.ID)
	if before.policyCalls != 3 || before.actions != 3 || before.transitions != 4 ||
		before.seals != 1 || before.mutations != 0 {
		t.Fatalf("repository cognition pre-replay durable counts=%+v", before)
	}
	prepared, generated, cleaned, attestations, prompts := client.prepared, client.generated,
		client.cleaned, client.attestations, len(client.prompts)
	if err := session.runRepositoryCognitionShadow(decision, indexed.Analyses[0].ID); err != nil {
		t.Fatalf("replay completed repository cognition: %v", err)
	}
	after := loadRepositoryCognitionCounts(t, ctx, pool, episodeID, claim.Job.ID)
	if after != before || client.prepared != prepared || client.generated != generated ||
		client.cleaned != cleaned || len(client.prompts) != prompts ||
		client.attestations != attestations+2 || client.plain != 0 {
		t.Fatalf("repository cognition replay durable/client before=%+v/%d/%d/%d/%d/%d after=%+v/%d/%d/%d/%d/%d",
			before, prepared, generated, cleaned, attestations, prompts,
			after, client.prepared, client.generated, client.cleaned, client.attestations, len(client.prompts))
	}
	assertRepositoryCognitionSealedTrace(t, ctx, repository, cognition.EpisodeID(episodeID), brain)
}
