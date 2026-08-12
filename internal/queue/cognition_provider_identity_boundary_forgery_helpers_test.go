package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
)

type providerBoundaryObservationMutation string

const (
	providerBoundaryNoncanonicalTimestamp providerBoundaryObservationMutation = "noncanonical_timestamp"
	providerBoundaryNullTimestamp         providerBoundaryObservationMutation = "null_timestamp"
)

func assertDirectReplayObservationForgeryRejected(
	t *testing.T, repository *Repository, fixture taskGenerationRetirementFixture,
	mutation providerBoundaryObservationMutation,
) {
	t.Helper()
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	bootstrap := freshReplayBrainBootstrap(t, fixture.Start.BrainBootstrap)
	activation := cognitionGuardProviderProcessActivationFor(
		t, t.Context(), fixture.EpisodeID, replacement, bootstrap.AttestedBrain,
	)
	receiptRaw := providerBoundaryCanonical(t, activation.Receipt)
	observation := providerBoundaryObject(t, bootstrap.AttestedBrain.BootstrapObservation)
	mutateProviderBoundaryObservation(t, observation,
		bootstrap.AttestedBrain.BootstrapObservation.ObservedAt, mutation)
	observationRaw := providerBoundaryCanonical(t, observation)
	authority := map[string]any{
		"schema": cognitionEpisodeReplayBootstrapSchemaV1, "episode_id": fixture.EpisodeID,
		"job_id": replacement.JobID, "generation": replacement.Generation,
		"step_id": replacement.StepID, "attempt": replacement.Attempt,
		"worker_id": replacement.WorkerID, "observation_sha256": observation["observation_sha256"],
		"evidence_id":            bootstrap.BootstrapEvidence.Ref.ID,
		"process_observation_id": activation.Receipt.ID,
		"process_receipt_sha256": cognitionPayloadSHA(receiptRaw),
		"process_evidence_id":    activation.IdentityEvidence.Ref.ID,
	}
	authorityRaw := providerBoundaryCanonical(t, authority)
	tx, err := repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = insertCognitionProviderIdentityEvidenceBodyTx(
		t.Context(), tx, bootstrap.BootstrapEvidence,
	); err == nil {
		_, err = tx.Exec(t.Context(), `INSERT INTO cognition_episode_replay_provider_identity_evidence (
			replay_id,episode_id,evidence_id,job_id,generation,step_id,step_attempt,worker_id,
			provider_observation_json,provider_observation_json_sha256,
			provider_observation_sha256,observed_at,process_observation_id,
			process_receipt_sha256,process_evidence_id,authority_json,authority_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			"cognition_replay_bootstrap_"+cognitionPayloadSHA(authorityRaw), fixture.EpisodeID,
			bootstrap.BootstrapEvidence.Ref.ID, replacement.JobID, replacement.Generation,
			replacement.StepID, replacement.Attempt, replacement.WorkerID,
			string(observationRaw), cognitionPayloadSHA(observationRaw), observation["observation_sha256"],
			bootstrap.AttestedBrain.BootstrapObservation.ObservedAt, activation.Receipt.ID,
			cognitionPayloadSHA(receiptRaw), activation.IdentityEvidence.Ref.ID,
			string(authorityRaw), cognitionPayloadSHA(authorityRaw))
	}
	if err == nil {
		err = persistCognitionProviderProcessActivationTx(
			t.Context(), tx, replacement,
			cognitionActiveReplayEpisode(fixture, replacement, bootstrap.AttestedBrain),
			activation, "",
		)
	}
	assertProviderBoundaryCommitRejected(t, tx, err)
}

func assertDirectProcessObservationForgeryRejected(
	t *testing.T, repository *Repository, fixture taskGenerationRetirementFixture, postseal bool,
	observationMutation providerBoundaryObservationMutation, stableMutation string,
) {
	t.Helper()
	if postseal {
		if _, err := repository.CancelCognitionEpisode(t.Context(), cognitionCancellationForTest(
			t, fixture, fmt.Errorf("seal before postseal boundary forgery"),
		)); err != nil {
			t.Fatal(err)
		}
	}
	activation := providerProcessReceiptForTest(t, fixture)
	payload := forgeProviderProcessBoundaryPayload(t, activation, observationMutation, stableMutation)
	tx, err := repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = insertCognitionProviderIdentityEvidenceBodyTx(
		t.Context(), tx, activation.IdentityEvidence,
	); err == nil {
		err = insertForgedProviderProcessBoundary(t, tx, fixture, activation, payload, postseal)
	}
	assertProviderBoundaryCommitRejected(t, tx, err)
}

type providerProcessBoundaryPayload struct {
	receiptRaw, stableRaw, observationRaw []byte
	receiptID, receiptSHA, stableSHA      string
	observationSHA, challengeSHA          string
}

func forgeProviderProcessBoundaryPayload(
	t *testing.T, activation cognitionpolicy.ProviderProcessActivation,
	observationMutation providerBoundaryObservationMutation, stableMutation string,
) providerProcessBoundaryPayload {
	t.Helper()
	receipt := providerBoundaryObject(t, activation.Receipt)
	stable := receipt["stable_brain"].(map[string]any)
	observation := receipt["observation"].(map[string]any)
	if observationMutation != "" {
		mutateProviderBoundaryObservation(t, observation, activation.Receipt.Observation.ObservedAt,
			observationMutation)
	}
	if stableMutation != "" {
		mutateProviderBoundaryStableBrain(t, stable, stableMutation)
		challenge := providerBoundaryProcessChallenge(t, receipt, stable)
		observation["challenge_sha256"] = challenge
		rehashProviderBoundaryObject(t, observation, "observation_sha256")
	}
	receipt["stable_brain"], receipt["observation"], receipt["id"] = stable, observation, ""
	empty := providerBoundaryCanonical(t, receipt)
	receiptID := "provider_process_observation_" + cognitionPayloadSHA(empty)
	receipt["id"] = receiptID
	receiptRaw := providerBoundaryCanonical(t, receipt)
	return providerProcessBoundaryPayload{
		receiptRaw: receiptRaw, stableRaw: providerBoundaryCanonical(t, stable),
		observationRaw: providerBoundaryCanonical(t, observation), receiptID: receiptID,
		receiptSHA: cognitionPayloadSHA(receiptRaw), stableSHA: stable["sha256"].(string),
		observationSHA: observation["observation_sha256"].(string),
		challengeSHA:   observation["challenge_sha256"].(string),
	}
}

func mutateProviderBoundaryObservation(
	t *testing.T, observation map[string]any, observedAt time.Time,
	mutation providerBoundaryObservationMutation,
) {
	t.Helper()
	switch mutation {
	case providerBoundaryNoncanonicalTimestamp:
		observation["observed_at"] = observedAt.UTC().Format("2006-01-02T15:04:05.0000000Z")
	case providerBoundaryNullTimestamp:
		observation["observed_at"] = nil
	default:
		t.Fatalf("unregistered provider observation mutation %q", mutation)
	}
	rehashProviderBoundaryObject(t, observation, "observation_sha256")
}

func mutateProviderBoundaryStableBrain(t *testing.T, stable map[string]any, mutation string) {
	t.Helper()
	switch mutation {
	case "schema":
		stable["schema"] = "omnidex.stable-brain-authority.forged"
		rehashProviderBoundaryObject(t, stable, "sha256")
	case "self_hash":
		stable["sha256"] = strings.Repeat("f", 64)
	case "substituted_hardware":
		brain := stable["brain"].(map[string]any)
		brain["hardware"] = brain["hardware"].(string) + "-forged"
		stable["brain"] = brain
		rehashProviderBoundaryObject(t, stable, "sha256")
	default:
		t.Fatalf("unregistered stable Brain mutation %q", mutation)
	}
}

func providerBoundaryProcessChallenge(
	t *testing.T, receipt, stable map[string]any,
) string {
	t.Helper()
	scope := map[string]any{"episode_id": receipt["episode_id"], "actor": receipt["actor"],
		"purpose": receipt["purpose"], "stable_brain_sha256": stable["sha256"]}
	var brain cognitionpolicy.BrainRef
	if err := json.Unmarshal(providerBoundaryCanonical(t, stable["brain"]), &brain); err != nil {
		t.Fatal(err)
	}
	expected, err := brain.ProviderExpectation()
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(
		"cognition-process:"+cognitionPayloadSHA(providerBoundaryCanonical(t, scope)), expected,
	)
	if err != nil {
		t.Fatal(err)
	}
	return challenge
}

func insertForgedProviderProcessBoundary(
	t *testing.T, tx pgx.Tx, fixture taskGenerationRetirementFixture,
	activation cognitionpolicy.ProviderProcessActivation, payload providerProcessBoundaryPayload,
	postseal bool,
) error {
	t.Helper()
	table := "cognition_provider_process_observations"
	var sequence int64
	if postseal {
		table = "cognition_provider_postseal_observations"
	}
	if err := tx.QueryRow(t.Context(), "SELECT COALESCE(MAX(sequence),0)+1 FROM "+table+
		" WHERE episode_id=$1", fixture.EpisodeID).Scan(&sequence); err != nil {
		return err
	}
	common := []any{payload.receiptID, activation.IdentityEvidence.Ref.ID, fixture.EpisodeID,
		fixture.Authority.JobID, fixture.Authority.Generation, fixture.Authority.StepID,
		fixture.Authority.Attempt, fixture.Authority.WorkerID, activation.Receipt.Purpose, sequence}
	if !postseal {
		_, err := tx.Exec(t.Context(), `INSERT INTO cognition_provider_process_observations (
			observation_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			purpose,sequence,stable_brain_json,stable_brain_json_sha256,stable_brain_sha256,
			provider_observation_json,provider_observation_json_sha256,provider_observation_sha256,
			provider_attestation_sha256,challenge_sha256,observed_at,receipt_json,receipt_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
			append(common, string(payload.stableRaw), cognitionPayloadSHA(payload.stableRaw), payload.stableSHA,
				string(payload.observationRaw), cognitionPayloadSHA(payload.observationRaw), payload.observationSHA,
				activation.Receipt.Observation.AttestationSHA256, payload.challengeSHA,
				activation.Receipt.Observation.ObservedAt, string(payload.receiptRaw), payload.receiptSHA)...)
		return err
	}
	var trace string
	if err := tx.QueryRow(t.Context(), `SELECT trace_sha256 FROM cognition_terminal_seals
		WHERE episode_id=$1`, fixture.EpisodeID).Scan(&trace); err != nil {
		return err
	}
	previous := trace
	if sequence > 1 {
		if err := tx.QueryRow(t.Context(), `SELECT chain_sha256 FROM cognition_provider_postseal_observations
			WHERE episode_id=$1 AND sequence=$2`, fixture.EpisodeID, sequence-1).Scan(&previous); err != nil {
			return err
		}
	}
	source := CognitionProviderPostSealDirectAudit
	chain := providerPostSealChainSHA(trace, previous, sequence, source, payload.receiptSHA)
	_, err := tx.Exec(t.Context(), `INSERT INTO cognition_provider_postseal_observations (
		observation_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
		purpose,sequence,source_kind,terminal_trace_sha256,previous_chain_sha256,chain_sha256,
		stable_brain_json,stable_brain_json_sha256,stable_brain_sha256,
		provider_observation_json,provider_observation_json_sha256,provider_observation_sha256,
		provider_attestation_sha256,challenge_sha256,observed_at,receipt_json,receipt_sha256
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`,
		append(common, source, trace, previous, chain, string(payload.stableRaw), cognitionPayloadSHA(payload.stableRaw),
			payload.stableSHA, string(payload.observationRaw), cognitionPayloadSHA(payload.observationRaw),
			payload.observationSHA, activation.Receipt.Observation.AttestationSHA256,
			payload.challengeSHA, activation.Receipt.Observation.ObservedAt,
			string(payload.receiptRaw), payload.receiptSHA)...)
	return err
}

func assertProviderBoundaryCommitRejected(t *testing.T, tx pgx.Tx, err error) {
	t.Helper()
	if err == nil {
		err = tx.Commit(t.Context())
	}
	if err == nil {
		t.Fatal("direct SQL committed a rehashed provider identity boundary forgery")
	}
}

func providerBoundaryObject(t *testing.T, value any) map[string]any {
	t.Helper()
	raw := providerBoundaryCanonical(t, value)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	object := map[string]any{}
	if err := decoder.Decode(&object); err != nil {
		t.Fatal(err)
	}
	return object
}

func rehashProviderBoundaryObject(t *testing.T, object map[string]any, key string) {
	t.Helper()
	object[key] = ""
	object[key] = cognitionPayloadSHA(providerBoundaryCanonical(t, object))
}

func providerBoundaryCanonical(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := exactjson.Canonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
