package queue

import (
	"context"
	"testing"
)

func TestPostgresCognitionFactAuthorityRejectsDirectEntryAndEvidenceForgery(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	facts := cognitionFactAuthorityForTest(t, planFirstCognitionObservation, cognitionTestDigest("a"))
	episode, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, facts)
	if err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(t.Context(), `
		INSERT INTO task_entries (
			ledger_id,job_id,id,scope_node_id,kind,status,authority,content,content_sha256,
			created_by,metadata,created_version,updated_version
		)
		SELECT ledger_id,job_id,'forged-cognition-fact',scope_node_id,kind,status,authority,
		       content,content_sha256,created_by,metadata,created_version,updated_version
		FROM task_entries WHERE ledger_id=$1 AND kind='fact'
	`, episode.LedgerID)
	assertCognitionTransactionRejected(t, t.Context(), tx, err, "direct code fact without normalized authority")

	tx, err = pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(t.Context(), `
		INSERT INTO cognition_accepted_fact_evidence (
			fact_id,position,observation_id,revision,revision_sha256,content_sha256
		)
		SELECT fact_id,1,'forged-observation',1,$2,$2
		FROM cognition_accepted_facts WHERE episode_id=$1
	`, fixture.EpisodeID, cognitionTestDigest("b"))
	assertCognitionTransactionRejected(t, t.Context(), tx, err, "accepted fact evidence append")
}
