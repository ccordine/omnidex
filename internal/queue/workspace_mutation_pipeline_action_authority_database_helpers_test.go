package queue

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/scrum"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workspaceMutationPipelineActionFixture struct {
	repository *Repository
	claim      *model.ClaimedStep
	command    WorkspaceMutationCommand
	identity   workspaceMutationOperationIdentity
}

func (fixture workspaceMutationPipelineActionFixture) commandDatabase() *pgxpool.Pool {
	return fixture.repository.pool
}

func newWorkspaceMutationPipelineActionFixture(
	t *testing.T,
	repository *Repository,
	pipeline string,
	label string,
) workspaceMutationPipelineActionFixture {
	t.Helper()
	root := t.TempDir()
	job, projectID := enqueueWorkspaceMutationPipelineActionJob(
		t, repository, pipeline, label, root,
	)
	claim, err := repository.ClaimNextStep(t.Context(), "workspace-mutation-168-"+label)
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("workspace mutation pipeline/action claim=%+v error=%v", claim, err)
	}
	wantAction := "v3_coding"
	if pipeline == model.PipelineChat {
		wantAction = "objective_resolve"
	}
	if claim.Job.Pipeline != pipeline || claim.Step.Action != wantAction {
		t.Fatalf(
			"workspace mutation pipeline/action claim=%s/%s want %s/%s",
			claim.Job.Pipeline, claim.Step.Action, pipeline, wantAction,
		)
	}

	source, err := workspacefacts.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		t.Context(), source, "objective_"+queueTestSHA256("workspace-mutation-168-"+label),
		[]workspacefacts.DesiredFileState{{
			Path: "generated/" + label + ".txt", Present: true,
			Content: []byte("generated " + label + "\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	verification, err := NewWorkspaceMutationVerificationPlan(
		[]WorkspaceMutationVerificationIntent{{
			Kind: evidence.KindTestResult, Command: "verify " + label,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	command := WorkspaceMutationCommand{
		JobID: claim.Job.ID, StepID: claim.Step.ID,
		Generation: claim.Authority.Generation, CreatorAttempt: claim.Authority.Attempt,
		CreatorWorkerID: claim.Authority.WorkerID, ProjectID: projectID,
		ProjectLocation: root,
		Plan:            plan, Verification: verification,
	}
	identity, err := workspaceMutationOperation(command)
	if err != nil {
		t.Fatal(err)
	}
	return workspaceMutationPipelineActionFixture{
		repository: repository, claim: claim, command: command, identity: identity,
	}
}

func enqueueWorkspaceMutationPipelineActionJob(
	t *testing.T,
	repository *Repository,
	pipeline string,
	label string,
	root string,
) (model.Job, int64) {
	t.Helper()
	ctx := t.Context()
	switch pipeline {
	case model.PipelineChat:
		channel, err := repository.CreateChannel(ctx, model.Channel{
			ID:    model.ChannelID("workspace-mutation-168-" + label),
			Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant,
			Name: "Workspace mutation " + label, WorkspaceRoot: root,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Mutate "+label)
		if err != nil {
			t.Fatal(err)
		}
		return job, channel.ProjectID
	case model.PipelineCoding, model.PipelineScrum:
		project, err := repository.CreateProject(ctx, "workspace-mutation-"+label, root, "")
		if err != nil {
			t.Fatal(err)
		}
		if pipeline == model.PipelineCoding {
			metadata, err := json.Marshal(map[string]any{
				"project_id": project.ID, "client_cwd": root,
			})
			if err != nil {
				t.Fatal(err)
			}
			job, err := repository.EnqueueJob(ctx, "Mutate "+label, pipeline, metadata)
			if err != nil {
				t.Fatal(err)
			}
			return job, project.ID
		}
		card, err := repository.CreateScrumCard(
			ctx, project.ID, "", "Workspace mutation "+label, "", "assigned", nil, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		job, err := repository.EnqueueScrumJob(ctx, "Mutate "+label, scrum.JobMetadata{
			Source: scrum.JobMetadataSource, ProjectID: project.ID, CardID: card.ID,
			CardTitle: card.Title, CardDescription: card.Description,
			ReturnColumn: card.Column, ModelConfig: modelconfig.Config{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return job, project.ID
	default:
		t.Fatalf("unsupported workspace mutation fixture pipeline %q", pipeline)
		return model.Job{}, 0
	}
}

func workspaceMutationInsertAuthorityCatalog(
	t *testing.T,
	pool *pgxpool.Pool,
) string {
	t.Helper()
	var catalog string
	if err := pool.QueryRow(t.Context(), `
		SELECT jsonb_build_object(
			'function_source',procedure.prosrc,
			'function_kind',procedure.prokind,
			'function_arguments',procedure.pronargs,
			'function_default_arguments',procedure.pronargdefaults,
			'function_return',procedure.prorettype::regtype::text,
			'function_language',language.lanname,
			'function_volatility',procedure.provolatile,
			'function_parallel',procedure.proparallel,
			'function_strict',procedure.proisstrict,
			'function_security_definer',procedure.prosecdef,
			'function_leakproof',procedure.proleakproof,
			'function_config',procedure.proconfig,
			'trigger_enabled',trigger_row.tgenabled,
			'trigger_type',trigger_row.tgtype,
			'trigger_attributes',trigger_row.tgattr::text,
			'trigger_qualification',trigger_row.tgqual,
			'trigger_constraint',trigger_row.tgconstraint,
			'trigger_constraint_relation',trigger_row.tgconstrrelid,
			'trigger_constraint_index',trigger_row.tgconstrindid,
			'trigger_deferrable',trigger_row.tgdeferrable,
			'trigger_initially_deferred',trigger_row.tginitdeferred,
			'trigger_arguments',trigger_row.tgnargs,
			'trigger_argument_bytes',octet_length(trigger_row.tgargs),
			'trigger_old_table',trigger_row.tgoldtable,
			'trigger_new_table',trigger_row.tgnewtable,
			'trigger_internal',trigger_row.tgisinternal
		)::text
		FROM pg_trigger AS trigger_row
		JOIN pg_proc AS procedure ON procedure.oid=trigger_row.tgfoid
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE trigger_row.tgrelid='workspace_mutation_operations'::regclass
		  AND trigger_row.tgname='workspace_mutation_insert_validate'
	`).Scan(&catalog); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func loadWorkspaceMutationInsertSourceSHA256(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var digest string
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
		FROM pg_proc WHERE oid='validate_workspace_mutation_insert()'::regprocedure
	`).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	return digest
}

func loadWorkspaceMutationCurrentSourceSHA256(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var digest string
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
		FROM pg_proc
		WHERE oid='workspace_mutation_current_authority_valid(workspace_mutation_operations)'::regprocedure
	`).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	return digest
}

func requireNoWorkspaceMutationOperation(
	t *testing.T,
	pool *pgxpool.Pool,
	operationID string,
) {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM workspace_mutation_operations WHERE id=$1
	`, operationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected workspace mutation persisted %d operations", count)
	}
}

func workspaceMutationPipelineActionLabel(pipeline, suffix string) string {
	return fmt.Sprintf("%s-%s", pipeline, suffix)
}
