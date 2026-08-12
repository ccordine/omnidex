package queue

import (
	"bytes"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

func TestPostgresCognitionProposalMaterializationRejectsCoherentForgeries(t *testing.T) {
	_, raw := cognitionProposalMaterializationFixture(t)
	original, err := DecodeCognitionProposalMaterialization(raw, cognitionPayloadSHA(raw))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		mutate func(*CognitionProposalMaterialization)
		rehash bool
	}{
		"source kind": {mutate: func(value *CognitionProposalMaterialization) {
			value.SourceKind = cognitionstate.SourceModelQuestion
		}, rehash: true},
		"command": {mutate: func(value *CognitionProposalMaterialization) {
			value.Command.Content = "changed proposal content"
		}, rehash: true},
		"entry": {mutate: func(value *CognitionProposalMaterialization) {
			value.EntryURI = "task:ledger/changed/entry/changed"
		}, rehash: true},
		"version": {mutate: func(value *CognitionProposalMaterialization) {
			value.OutputLedgerVersion++
		}, rehash: true},
		"reordered proposal": {mutate: func(value *CognitionProposalMaterialization) {
			value.ProposalIndex++
		}, rehash: true},
		"extra proposal": {mutate: func(value *CognitionProposalMaterialization) {
			value.ProposalIndex = 31
		}, rehash: true},
		"revision proposal": {mutate: func(value *CognitionProposalMaterialization) {
			value.Proposal.Kind = cognition.ProposalRevision
		}, rehash: true},
		"embedded ledger": {mutate: func(value *CognitionProposalMaterialization) {
			value.PreProposalLedger.Version++
		}, rehash: true},
		"self hash": {mutate: func(value *CognitionProposalMaterialization) {
			value.SHA256 = cognitionTestDigest("f")
			value.ID = cognitionProposalMaterializationPrefix + value.SHA256
		}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			forged := original
			test.mutate(&forged)
			if test.rehash {
				forged.SHA256, err = cognitionProposalCanonicalSHA(forged.identity())
				if err != nil {
					t.Fatal(err)
				}
				forged.ID = cognitionProposalMaterializationPrefix + forged.SHA256
			}
			changed, err := exactjson.Canonical(forged)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(raw, changed) {
				t.Fatal("forgery did not change proposal materialization")
			}
			if _, err := DecodeCognitionProposalMaterialization(
				changed, cognitionPayloadSHA(changed),
			); err == nil {
				t.Fatal("self-consistently rehashed proposal materialization forgery was accepted")
			}
		})
	}
}

func TestPostgresCognitionProposalMaterializationReverseTotalityRejectsOmission(t *testing.T) {
	fixture, _ := cognitionProposalMaterializationFixture(t)
	tx, err := fixture.Repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `
		ALTER TABLE cognition_proposal_materializations
		DISABLE TRIGGER cognition_proposal_materializations_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		DELETE FROM cognition_proposal_materializations WHERE episode_id=$1
	`, fixture.EpisodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); err == nil {
		t.Fatal("reconciliation committed without its proposal materialization")
	}
}

func cognitionProposalMaterializationFixture(
	t *testing.T,
) (cognitionDatabaseFixture, []byte) {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "063")); err != nil {
		t.Fatal(err)
	}
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		t.Context(), fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	decision := cognitionProposalMaterializationDecision(fixture)
	bound := buildCognitionDecisionStep(t, fixture, decision)
	if _, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), bound.Command); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := pool.QueryRow(t.Context(), `
		SELECT payload_json FROM cognition_proposal_materializations WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return fixture, raw
}

func expectProposalMaterializationCommitFailure(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Commit(t.Context()); err == nil {
		t.Fatal("forged proposal materialization transaction committed")
	}
}
