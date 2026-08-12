package queue

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const cognitionProviderFailureAuthoritySchemaV1 = "omnidex.cognition-provider-failure-authority.v1"

type cognitionProviderFailureKind string

const (
	cognitionProviderFailureBootstrap cognitionProviderFailureKind = "brain_bootstrap"
	cognitionProviderFailureProcess   cognitionProviderFailureKind = "provider_process"
)

type cognitionProviderFailureAuthority struct {
	Schema               string                       `json:"schema"`
	RecordID             string                       `json:"record_id"`
	FailureKind          cognitionProviderFailureKind `json:"failure_kind"`
	FailureID            string                       `json:"failure_id"`
	EpisodeID            cognition.EpisodeID          `json:"episode_id"`
	Actor                cognition.AttemptRef         `json:"actor"`
	EvidenceID           string                       `json:"evidence_id"`
	ReceiptSHA256        string                       `json:"receipt_sha256"`
	BootstrapEvidenceID  string                       `json:"bootstrap_evidence_id"`
	BootstrapBrainSHA256 string                       `json:"bootstrap_brain_sha256"`
}

type cognitionProviderFailureBootstrapBundle struct {
	BrainJSON []byte
	BrainSHA  string
	Evidence  llm.ProviderIdentityEvidence
}

func (r *Repository) RecordCognitionBrainBootstrapFailure(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	failure cognitionpolicy.BrainBootstrapFailure,
) error {
	if err := failure.Validate(); err != nil {
		return err
	}
	return r.recordCognitionProviderActivationFailure(
		ctx, authority, episodeID, cognitionProviderFailureBootstrap,
		failure.Receipt.ID, failure.Receipt, failure.IdentityEvidence, nil,
	)
}

func (r *Repository) RecordCognitionProviderProcessFailure(
	ctx context.Context,
	bootstrap cognitionpolicy.BrainBootstrap,
	failure cognitionpolicy.ProviderProcessFailure,
) error {
	if err := bootstrap.Validate(); err != nil {
		return err
	}
	if err := failure.ValidateFor(bootstrap.AttestedBrain); err != nil {
		return err
	}
	authority, err := providerProcessObservationAuthority(failure.Receipt.Actor)
	if err != nil {
		return err
	}
	return r.recordCognitionProviderActivationFailure(
		ctx, authority, failure.Receipt.EpisodeID, cognitionProviderFailureProcess,
		failure.Receipt.ID, failure.Receipt, failure.IdentityEvidence, &bootstrap,
	)
}

func (r *Repository) recordCognitionProviderActivationFailure(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	kind cognitionProviderFailureKind,
	failureID string,
	receipt any,
	evidence llm.ProviderIdentityEvidence,
	bootstrap *cognitionpolicy.BrainBootstrap,
) error {
	if r == nil || r.pool == nil || ctx == nil {
		return fmt.Errorf("provider activation failure requires PostgreSQL and context")
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return err
	}
	if err := cognitionEpisodeIdentityValid(episodeID); err != nil {
		return err
	}
	if err := evidence.Validate(); err != nil {
		return err
	}
	if (kind == cognitionProviderFailureBootstrap) != (bootstrap == nil) {
		return fmt.Errorf("provider activation failure kind changed its bootstrap evidence contract")
	}
	receiptJSON, err := exactjson.Canonical(receipt)
	if err != nil {
		return err
	}
	receiptSHA := cognitionPayloadSHA(receiptJSON)
	bootstrapEvidence, err := prepareProviderFailureBootstrap(bootstrap)
	if err != nil {
		return err
	}
	bound, authorityJSON, err := newCognitionProviderFailureAuthority(
		kind, failureID, episodeID, authority, evidence.Ref.ID, receiptSHA,
		bootstrapEvidence,
	)
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.AuthorizeStepAttemptTransaction(ctx, tx, authority); err != nil {
		return err
	}
	found, err := exactCognitionProviderFailureReplayTx(
		ctx, tx, bound, receiptJSON, authorityJSON, evidence, bootstrapEvidence,
	)
	if err != nil {
		return err
	}
	if found {
		return tx.Commit(ctx)
	}
	if err := insertCognitionProviderIdentityEvidenceBodyTx(ctx, tx, evidence); err != nil {
		return err
	}
	if bootstrap != nil {
		if err := insertCognitionProviderIdentityEvidenceBodyTx(
			ctx, tx, bootstrapEvidence.Evidence,
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_provider_activation_failures (
			record_id,failure_kind,failure_id,episode_id,evidence_id,
			bootstrap_evidence_id,bootstrap_brain_json,bootstrap_brain_sha256,
			job_id,generation,step_id,step_attempt,worker_id,
			receipt_json,receipt_sha256,authority_json,authority_sha256
		) VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),
		          $9,$10,$11,$12,$13,$14,$15,$16,$17)
	`, bound.RecordID, kind, failureID, episodeID, evidence.Ref.ID,
		bound.BootstrapEvidenceID, string(bootstrapEvidence.BrainJSON), bootstrapEvidence.BrainSHA,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt,
		authority.WorkerID, string(receiptJSON), receiptSHA, string(authorityJSON),
		cognitionPayloadSHA(authorityJSON)); err != nil {
		return fmt.Errorf("persist provider activation failure %q: %w", bound.RecordID, err)
	}
	if err := terminalizeCognitionProviderActivationFailureTx(
		ctx, tx, authority, episodeID, bound.RecordID,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func newCognitionProviderFailureAuthority(
	kind cognitionProviderFailureKind,
	failureID string,
	episodeID cognition.EpisodeID,
	authority model.StepAttemptAuthority,
	evidenceID string,
	receiptSHA string,
	bootstrap cognitionProviderFailureBootstrapBundle,
) (cognitionProviderFailureAuthority, []byte, error) {
	value := cognitionProviderFailureAuthority{
		Schema: cognitionProviderFailureAuthoritySchemaV1, FailureKind: kind,
		FailureID: failureID, EpisodeID: episodeID, Actor: cognitionAttempt(authority),
		EvidenceID: evidenceID, ReceiptSHA256: receiptSHA,
		BootstrapEvidenceID:  bootstrap.Evidence.Ref.ID,
		BootstrapBrainSHA256: bootstrap.BrainSHA,
	}
	raw, err := exactjson.Canonical(value)
	if err != nil {
		return cognitionProviderFailureAuthority{}, nil, err
	}
	value.RecordID = "cognition_provider_failure_" + cognitionPayloadSHA(raw)
	raw, err = exactjson.Canonical(value)
	return value, raw, err
}

func exactCognitionProviderFailureReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	authority cognitionProviderFailureAuthority,
	receiptJSON, authorityJSON []byte,
	evidence llm.ProviderIdentityEvidence,
	bootstrap cognitionProviderFailureBootstrapBundle,
) (bool, error) {
	var gotReceipt, gotAuthority string
	var gotBootstrapEvidenceID, gotBootstrapBrain, gotBootstrapBrainSHA *string
	err := tx.QueryRow(ctx, `
		SELECT receipt_json,authority_json,bootstrap_evidence_id,
		       bootstrap_brain_json,bootstrap_brain_sha256
		FROM cognition_provider_activation_failures
		WHERE record_id=$1
	`, authority.RecordID).Scan(
		&gotReceipt, &gotAuthority, &gotBootstrapEvidenceID,
		&gotBootstrapBrain, &gotBootstrapBrainSHA,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if gotReceipt != string(receiptJSON) || gotAuthority != string(authorityJSON) {
		return false, fmt.Errorf("%w: provider activation failure replay changed", ErrCognitionConflict)
	}
	if !exactOptionalProviderFailureBootstrap(
		gotBootstrapEvidenceID, gotBootstrapBrain, gotBootstrapBrainSHA, bootstrap,
	) {
		return false, fmt.Errorf("%w: provider activation bootstrap replay changed", ErrCognitionConflict)
	}
	persisted, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, evidence.Ref.ID)
	if err != nil || !reflect.DeepEqual(persisted, evidence) {
		return false, fmt.Errorf(
			"%w: provider activation failure raw evidence replay changed: %v",
			ErrCognitionConflict, err,
		)
	}
	if bootstrap.Evidence.Ref.ID != "" {
		persisted, err = loadCognitionProviderIdentityEvidenceTx(
			ctx, tx, bootstrap.Evidence.Ref.ID,
		)
		if err != nil || !reflect.DeepEqual(persisted, bootstrap.Evidence) {
			return false, fmt.Errorf(
				"%w: provider activation bootstrap evidence replay changed: %v",
				ErrCognitionConflict, err,
			)
		}
	}
	return true, nil
}

func prepareProviderFailureBootstrap(
	bootstrap *cognitionpolicy.BrainBootstrap,
) (cognitionProviderFailureBootstrapBundle, error) {
	if bootstrap == nil {
		return cognitionProviderFailureBootstrapBundle{}, nil
	}
	if err := bootstrap.Validate(); err != nil {
		return cognitionProviderFailureBootstrapBundle{}, err
	}
	raw, err := exactjson.Canonical(bootstrap.AttestedBrain)
	if err != nil {
		return cognitionProviderFailureBootstrapBundle{}, err
	}
	return cognitionProviderFailureBootstrapBundle{
		BrainJSON: raw, BrainSHA: cognitionPayloadSHA(raw),
		Evidence: bootstrap.BootstrapEvidence.Clone(),
	}, nil
}

func exactOptionalProviderFailureBootstrap(
	evidenceID, brainJSON, brainSHA *string,
	bootstrap cognitionProviderFailureBootstrapBundle,
) bool {
	if bootstrap.Evidence.Ref.ID == "" {
		return evidenceID == nil && brainJSON == nil && brainSHA == nil
	}
	return evidenceID != nil && *evidenceID == bootstrap.Evidence.Ref.ID &&
		brainJSON != nil && *brainJSON == string(bootstrap.BrainJSON) &&
		brainSHA != nil && *brainSHA == bootstrap.BrainSHA
}
