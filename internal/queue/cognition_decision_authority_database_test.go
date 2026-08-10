package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCognitionReconciliationAcceptsSelectedActionThroughCode(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	_, _ = prepareCognitionProposalAction(t, fixture)
	var authorities, candidates, accepted int
	if err := pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT COUNT(*) FROM cognition_decision_acceptances WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM task_entries WHERE ledger_id=(
		    SELECT ledger_id FROM cognition_episodes WHERE episode_id=$1)
		    AND metadata->>'source_kind'='model_decision_candidate' AND status='superseded'),
		 (SELECT COUNT(*) FROM task_entries WHERE ledger_id=(
		    SELECT ledger_id FROM cognition_episodes WHERE episode_id=$1)
		    AND kind='accepted_decision'
		    AND acceptance_policy='cognition-policy-call-and-action-schema-v1')
	`, fixture.EpisodeID).Scan(
		&authorities, &candidates, &accepted,
	); err != nil {
		t.Fatal(err)
	}
	if authorities != 1 || candidates != 1 || accepted != 1 {
		t.Fatalf("selected decision authority/candidate/accepted=%d/%d/%d", authorities, candidates, accepted)
	}
}

func TestPostgresCognitionDecisionAuthorityRejectsForgedAcceptedEntry(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	_, _ = prepareCognitionProposalAction(t, fixture)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	_, err = tx.Exec(t.Context(), `
		INSERT INTO task_entries (
		 ledger_id,job_id,id,scope_node_id,kind,status,authority,content,content_sha256,
		 created_by,created_step_id,source_entry_id,acceptance_policy,accepted_by,metadata,
		 created_version,updated_version
		)
		SELECT candidate.ledger_id,candidate.job_id,'forged-cognition-accepted-decision',
		 candidate.scope_node_id,'accepted_decision','active','accepted_model_decision',
		 candidate.content,candidate.content_sha256,'code',candidate.created_step_id,candidate.id,
		 'cognition-policy-call-and-action-schema-v1','code',
		 '{"schema":"omnidex.cognition-decision-acceptance.v1"}'::jsonb,
		 candidate.updated_version+100,candidate.updated_version+100
		FROM task_entries candidate
		WHERE candidate.ledger_id=(SELECT ledger_id FROM cognition_episodes WHERE episode_id=$1)
		  AND candidate.metadata->>'source_kind'='model_obligation_candidate'
	`, fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "selected cognition accepted entry lacks normalized authority") {
		t.Fatalf("forged accepted entry commit error=%v", err)
	}
}

func TestPostgresTerminalTransitionRejectsUnappliedObligationCandidate(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	action, _ := prepareCognitionProposalAction(t, fixture)
	transition := cognitionProposalTransition(t, fixture, action)
	transition.Terminal = true
	transition.PublicOutcome = "The environment reached its terminal state."
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	graph, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Version != 1 || len(graph.Graph.Obligations) != 1 ||
		graph.Graph.Obligations[0].Status != cognition.ObligationActive {
		t.Fatalf("terminal transition incorrectly materialized graph: %+v", graph)
	}
	var dispositions, applications, rejected int
	if err := pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT COUNT(*) FROM cognition_proposal_dispositions
		    WHERE episode_id=$1 AND outcome='rejected_terminal_transition'),
		 (SELECT COUNT(*) FROM cognition_obligation_materialization_applications WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM task_entries WHERE ledger_id=(
		    SELECT ledger_id FROM cognition_episodes WHERE episode_id=$1)
		    AND metadata->>'source_kind'='model_obligation_candidate' AND status='rejected'
		    AND disposition_by=$2)
	`, fixture.EpisodeID, taskstate.AuthorityCode).Scan(
		&dispositions, &applications, &rejected,
	); err != nil {
		t.Fatal(err)
	}
	if dispositions != 1 || applications != 0 || rejected != 1 {
		t.Fatalf("terminal disposition/applications/rejected=%d/%d/%d", dispositions, applications, rejected)
	}
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatalf("exact terminal transition replay: %v", err)
	}
}
