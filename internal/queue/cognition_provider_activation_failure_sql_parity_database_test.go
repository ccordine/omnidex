package queue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type directProviderFailureRow struct {
	kind                cognitionProviderFailureKind
	failureID, recordID string
	episodeID           cognition.EpisodeID
	receipt, authority  map[string]any
	receiptRaw, authRaw []byte
	receiptSHA, authSHA string
	evidence            llm.ProviderIdentityEvidence
	bootstrap           cognitionProviderFailureBootstrapBundle
	createdAt           time.Time
}

func TestPostgresProviderFailureRejectsRehashedSemanticForgeries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *directProviderFailureRow)
	}{
		{"arbitrary self-consistent IDs", func(t *testing.T, row *directProviderFailureRow) {
			row.failureID = "provider_process_failure_" + strings.Repeat("a", 64)
			row.recordID = "cognition_provider_failure_" + strings.Repeat("b", 64)
			row.receipt["id"] = row.failureID
			row.authority["failure_id"], row.authority["record_id"] = row.failureID, row.recordID
			row.refreshBytes(t)
		}},
		{"unbounded label with nonempty proof", func(t *testing.T, row *directProviderFailureRow) {
			row.receipt["code"] = "provider_metadata_unbounded"
			row.receipt["live_host_hardware_attestation"] = cognitionpolicy.HostHardwareAttestation{}
			row.rederive(t)
		}},
		{"host mismatch with empty live host", func(t *testing.T, row *directProviderFailureRow) {
			row.receipt["live_host_hardware_attestation"] = cognitionpolicy.HostHardwareAttestation{}
			row.rederive(t)
		}},
		{"host mismatch with stored live host", func(t *testing.T, row *directProviderFailureRow) {
			stable := row.receipt["stable_brain"].(map[string]any)
			row.receipt["live_host_hardware_attestation"] = stable["host_hardware_attestation"]
			row.rederive(t)
		}},
		{"bootstrap Brain with extra authority", func(t *testing.T, row *directProviderFailureRow) {
			var brain map[string]any
			if err := json.Unmarshal(row.bootstrap.BrainJSON, &brain); err != nil {
				t.Fatal(err)
			}
			brain["forged"] = true
			row.bootstrap.BrainJSON = mustCanonicalProviderFailureValue(t, brain)
			row.bootstrap.BrainSHA = cognitionPayloadSHA(row.bootstrap.BrainJSON)
			row.authority["bootstrap_brain_sha256"] = row.bootstrap.BrainSHA
			row.rederiveAuthority(t)
		}},
		{"future evidence timestamp", func(_ *testing.T, row *directProviderFailureRow) {
			row.createdAt = time.Now().UTC().Add(time.Hour)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, pool, ctx := policyInputFreshRepository(t)
			fixture := prepareTaskGenerationRetirementFixture(
				t, repository, pool, ctx, "provider-failure-sql-"+strings.ReplaceAll(test.name, " ", "-"),
			)
			bootstrap := cognitionTestBrainBootstrapWithCPU("d")
			outcome, err := cognitionpolicy.ObserveProviderProcess(
				ctx, cognitionGuardPolicyClient{}, bootstrap.AttestedBrain,
				cognition.EpisodeRef{ID: fixture.EpisodeID}, cognitionAttempt(fixture.Authority),
				cognitionpolicy.ProviderProcessEpisodeInvocation,
			)
			if err == nil || outcome.Failure == nil ||
				outcome.Failure.Receipt.Code != cognitionpolicy.ProviderHostIdentityMismatch {
				t.Fatalf("host mismatch fixture outcome=%+v error=%v", outcome, err)
			}
			row := newDirectProviderFailureRow(
				t, fixture.Authority, fixture.EpisodeID, bootstrap, *outcome.Failure,
			)
			test.mutate(t, &row)
			assertDirectProviderFailureRejected(t, repository, fixture.Authority, row)
		})
	}
}

func TestPostgresProviderFailureUsesOneFinalExactTrigger(t *testing.T) {
	_, pool, ctx := policyInputFreshRepository(t)
	var count int
	var definition string
	if err := pool.QueryRow(ctx, `SELECT COUNT(*),max(pg_get_functiondef(p.oid))
		FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid
		WHERE t.tgrelid='cognition_provider_activation_failures'::regclass
		  AND t.tgname='cognition_provider_activation_failures_exact'
		  AND NOT t.tgisinternal`).Scan(&count, &definition); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"cognition_provider_failure_code_is_exact", "cognition_provider_process_challenge",
		"provider_attestation_mismatch", "cognition_empty_host_attestation",
	} {
		if count != 1 || !strings.Contains(definition, required) {
			t.Fatalf("failure exact trigger count=%d lacks %q", count, required)
		}
	}
}

func newDirectProviderFailureRow(
	t *testing.T,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	bootstrap cognitionpolicy.BrainBootstrap,
	failure cognitionpolicy.ProviderProcessFailure,
) directProviderFailureRow {
	t.Helper()
	bundle, err := prepareProviderFailureBootstrap(&bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	receipt := providerFailureObject(t, failure.Receipt)
	row := directProviderFailureRow{
		kind: cognitionProviderFailureProcess, episodeID: episodeID,
		receipt: receipt, evidence: failure.IdentityEvidence, bootstrap: bundle,
		createdAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	row.authority = map[string]any{
		"schema": cognitionProviderFailureAuthoritySchemaV1, "record_id": "",
		"failure_kind": string(row.kind), "failure_id": "", "episode_id": episodeID,
		"actor": cognitionAttempt(authority), "evidence_id": failure.IdentityEvidence.Ref.ID,
		"receipt_sha256": "", "bootstrap_evidence_id": bundle.Evidence.Ref.ID,
		"bootstrap_brain_sha256": bundle.BrainSHA,
	}
	row.rederive(t)
	return row
}

func (row *directProviderFailureRow) rederive(t *testing.T) {
	t.Helper()
	row.receipt["id"] = ""
	empty := mustCanonicalProviderFailureValue(t, row.receipt)
	row.failureID = "provider_process_failure_" + cognitionPayloadSHA(empty)
	row.receipt["id"] = row.failureID
	row.authority["failure_id"] = row.failureID
	row.refreshBytes(t)
	row.rederiveAuthority(t)
}

func (row *directProviderFailureRow) rederiveAuthority(t *testing.T) {
	t.Helper()
	row.authority["receipt_sha256"] = row.receiptSHA
	row.authority["record_id"] = ""
	empty := mustCanonicalProviderFailureValue(t, row.authority)
	row.recordID = "cognition_provider_failure_" + cognitionPayloadSHA(empty)
	row.authority["record_id"] = row.recordID
	row.authRaw = mustCanonicalProviderFailureValue(t, row.authority)
	row.authSHA = cognitionPayloadSHA(row.authRaw)
}

func (row *directProviderFailureRow) refreshBytes(t *testing.T) {
	t.Helper()
	row.receiptRaw = mustCanonicalProviderFailureValue(t, row.receipt)
	row.receiptSHA = cognitionPayloadSHA(row.receiptRaw)
	row.authority["receipt_sha256"] = row.receiptSHA
	row.authRaw = mustCanonicalProviderFailureValue(t, row.authority)
	row.authSHA = cognitionPayloadSHA(row.authRaw)
}

func providerFailureObject(t *testing.T, value any) map[string]any {
	t.Helper()
	raw := mustCanonicalProviderFailureValue(t, value)
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func mustCanonicalProviderFailureValue(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := exactjson.Canonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertDirectProviderFailureRejected(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	row directProviderFailureRow,
) {
	t.Helper()
	ctx := t.Context()
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := repository.AuthorizeStepAttemptTransaction(ctx, tx, authority); err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []llm.ProviderIdentityEvidence{row.evidence, row.bootstrap.Evidence} {
		if err := insertCognitionProviderIdentityEvidenceBodyTx(ctx, tx, evidence); err != nil {
			t.Fatal(err)
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO cognition_provider_activation_failures (
		record_id,failure_kind,failure_id,episode_id,evidence_id,
		bootstrap_evidence_id,bootstrap_brain_json,bootstrap_brain_sha256,
		job_id,generation,step_id,step_attempt,worker_id,
		receipt_json,receipt_sha256,authority_json,authority_sha256,created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		row.recordID, row.kind, row.failureID, row.episodeID, row.evidence.Ref.ID,
		row.bootstrap.Evidence.Ref.ID, string(row.bootstrap.BrainJSON), row.bootstrap.BrainSHA,
		authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID,
		string(row.receiptRaw), row.receiptSHA, string(row.authRaw), row.authSHA, row.createdAt)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err == nil {
		t.Fatal("direct SQL committed a forged provider activation failure")
	}
}
