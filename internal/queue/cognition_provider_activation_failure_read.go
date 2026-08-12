package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReadCognitionProviderActivationFailurePage(
	ctx context.Context,
	request CognitionProviderActivationFailurePageRequest,
) (CognitionProviderActivationFailurePage, error) {
	if r == nil || r.pool == nil || ctx == nil || request.validate() != nil {
		return CognitionProviderActivationFailurePage{},
			fmt.Errorf("provider activation failure page requires PostgreSQL and a bounded scope")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return CognitionProviderActivationFailurePage{}, err
	}
	defer tx.Rollback(ctx)
	page, err := readCognitionProviderActivationFailurePageTx(ctx, tx, request)
	if err != nil {
		return CognitionProviderActivationFailurePage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionProviderActivationFailurePage{}, err
	}
	return page, nil
}

type persistedProviderActivationFailure struct {
	record              CognitionProviderActivationFailureRecord
	failureID           string
	evidenceID          string
	receipt             []byte
	authority           []byte
	receiptSHA          string
	authoritySHA        string
	bootstrapEvidenceID *string
	bootstrapBrain      *string
	bootstrapBrainSHA   *string
}

func readCognitionProviderActivationFailurePageTx(
	ctx context.Context,
	tx pgx.Tx,
	request CognitionProviderActivationFailurePageRequest,
) (CognitionProviderActivationFailurePage, error) {
	page := CognitionProviderActivationFailurePage{NextRecordNumber: request.AfterRecordNumber}
	authority := request.Authority
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM cognition_provider_activation_failures
		WHERE episode_id=$1 AND job_id=$2 AND generation=$3 AND step_id=$4
		  AND step_attempt=$5 AND worker_id=$6`, request.EpisodeID, authority.JobID,
		authority.Generation, authority.StepID, authority.Attempt, authority.WorkerID,
	).Scan(&page.TotalRecords); err != nil {
		return CognitionProviderActivationFailurePage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT record_number,record_id,failure_kind,failure_id,
		evidence_id,receipt_json,receipt_sha256,authority_json,authority_sha256,created_at
		       ,bootstrap_evidence_id,bootstrap_brain_json,bootstrap_brain_sha256
		FROM cognition_provider_activation_failures
		WHERE episode_id=$1 AND job_id=$2 AND generation=$3 AND step_id=$4
		  AND step_attempt=$5 AND worker_id=$6 AND record_number>$7
		ORDER BY record_number LIMIT $8`, request.EpisodeID, authority.JobID,
		authority.Generation, authority.StepID, authority.Attempt, authority.WorkerID,
		request.AfterRecordNumber, request.Limit)
	if err != nil {
		return CognitionProviderActivationFailurePage{}, err
	}
	persisted := make([]persistedProviderActivationFailure, 0, request.Limit)
	for rows.Next() {
		value := persistedProviderActivationFailure{}
		if err := rows.Scan(&value.record.RecordNumber, &value.record.RecordID,
			&value.record.Kind, &value.failureID, &value.evidenceID, &value.receipt,
			&value.receiptSHA, &value.authority, &value.authoritySHA,
			&value.record.CreatedAt, &value.bootstrapEvidenceID,
			&value.bootstrapBrain, &value.bootstrapBrainSHA); err != nil {
			rows.Close()
			return CognitionProviderActivationFailurePage{}, err
		}
		persisted = append(persisted, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CognitionProviderActivationFailurePage{}, err
	}
	rows.Close()
	for _, value := range persisted {
		record, err := restoreProviderActivationFailureRecord(ctx, tx, request, value)
		if err != nil {
			return CognitionProviderActivationFailurePage{}, err
		}
		if record.RecordNumber <= page.NextRecordNumber {
			return CognitionProviderActivationFailurePage{},
				fmt.Errorf("%w: provider activation failure cursor changed", ErrCognitionConflict)
		}
		page.NextRecordNumber = record.RecordNumber
		page.Records = append(page.Records, record)
	}
	return page, nil
}

func restoreProviderActivationFailureRecord(
	ctx context.Context,
	tx pgx.Tx,
	request CognitionProviderActivationFailurePageRequest,
	value persistedProviderActivationFailure,
) (CognitionProviderActivationFailureRecord, error) {
	record := value.record
	evidence, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, value.evidenceID)
	if err != nil {
		return record, err
	}
	var persistedAuthority cognitionProviderFailureAuthority
	if err := cognitionDecodeExact(value.authority, &persistedAuthority); err != nil {
		return record, err
	}
	bootstrap, err := restoreProviderFailureBootstrap(ctx, tx, value)
	if err != nil {
		return record, err
	}
	want, wantJSON, err := newCognitionProviderFailureAuthority(
		cognitionProviderFailureKind(record.Kind), value.failureID, request.EpisodeID,
		request.Authority, value.evidenceID, value.receiptSHA, bootstrap,
	)
	if err != nil || persistedAuthority != want || string(value.authority) != string(wantJSON) ||
		cognitionPayloadSHA(value.authority) != value.authoritySHA ||
		cognitionPayloadSHA(value.receipt) != value.receiptSHA {
		return record, fmt.Errorf("%w: provider activation failure authority changed", ErrCognitionConflict)
	}
	record.EpisodeID, record.Actor = request.EpisodeID, cognitionAttempt(request.Authority)
	record.ReceiptSHA256, record.AuthoritySHA256 = value.receiptSHA, value.authoritySHA
	record.ReceiptJSON = append([]byte{}, value.receipt...)
	record.AuthorityJSON = append([]byte{}, value.authority...)
	switch cognitionProviderFailureKind(record.Kind) {
	case cognitionProviderFailureBootstrap:
		if bootstrap.Evidence.Ref.ID != "" {
			return record, fmt.Errorf("%w: bootstrap failure claims successful bootstrap evidence", ErrCognitionConflict)
		}
		var receipt cognitionpolicy.BrainBootstrapFailureReceipt
		if err := decodeProviderFailureReceipt(value.receipt, &receipt); err != nil {
			return record, err
		}
		failure := cognitionpolicy.BrainBootstrapFailure{Receipt: receipt, IdentityEvidence: evidence}
		if err := failure.Validate(); err != nil {
			return record, fmt.Errorf("%w: stored Brain bootstrap failure changed: %v", ErrCognitionConflict, err)
		}
		record.Bootstrap = &receipt
	case cognitionProviderFailureProcess:
		if bootstrap.Evidence.Ref.ID == "" {
			return record, fmt.Errorf("%w: process failure lost successful bootstrap evidence", ErrCognitionConflict)
		}
		var receipt cognitionpolicy.ProviderProcessFailureReceipt
		if err := decodeProviderFailureReceipt(value.receipt, &receipt); err != nil {
			return record, err
		}
		failure := cognitionpolicy.ProviderProcessFailure{Receipt: receipt, IdentityEvidence: evidence}
		if err := failure.ValidateFor(bootstrapBrain(bootstrap)); err != nil {
			return record, fmt.Errorf("%w: stored provider process failure changed: %v", ErrCognitionConflict, err)
		}
		record.Process = &receipt
	default:
		return record, fmt.Errorf("%w: provider activation failure kind changed", ErrCognitionConflict)
	}
	record.Evidence = providerFailureEvidenceManifest(evidence)
	if bootstrap.Evidence.Ref.ID != "" {
		brain := bootstrapBrain(bootstrap)
		manifest := providerFailureEvidenceManifest(bootstrap.Evidence)
		record.SuccessfulBootstrap, record.BootstrapEvidence = &brain, &manifest
	}
	return record, nil
}

func restoreProviderFailureBootstrap(
	ctx context.Context,
	tx pgx.Tx,
	value persistedProviderActivationFailure,
) (cognitionProviderFailureBootstrapBundle, error) {
	if value.bootstrapEvidenceID == nil && value.bootstrapBrain == nil &&
		value.bootstrapBrainSHA == nil {
		return cognitionProviderFailureBootstrapBundle{}, nil
	}
	if value.bootstrapEvidenceID == nil || value.bootstrapBrain == nil ||
		value.bootstrapBrainSHA == nil {
		return cognitionProviderFailureBootstrapBundle{},
			fmt.Errorf("%w: process failure bootstrap projection is partial", ErrCognitionConflict)
	}
	var brain cognitionpolicy.AttestedBrain
	if err := cognitionDecodeExact([]byte(*value.bootstrapBrain), &brain); err != nil {
		return cognitionProviderFailureBootstrapBundle{}, err
	}
	evidence, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, *value.bootstrapEvidenceID)
	if err != nil {
		return cognitionProviderFailureBootstrapBundle{}, err
	}
	bootstrap := cognitionpolicy.BrainBootstrap{AttestedBrain: brain, BootstrapEvidence: evidence}
	if bootstrap.Validate() != nil || cognitionPayloadSHA([]byte(*value.bootstrapBrain)) !=
		*value.bootstrapBrainSHA {
		return cognitionProviderFailureBootstrapBundle{},
			fmt.Errorf("%w: process failure bootstrap evidence changed", ErrCognitionConflict)
	}
	return cognitionProviderFailureBootstrapBundle{
		BrainJSON: []byte(*value.bootstrapBrain), BrainSHA: *value.bootstrapBrainSHA,
		Evidence: evidence,
	}, nil
}

func bootstrapBrain(bundle cognitionProviderFailureBootstrapBundle) cognitionpolicy.AttestedBrain {
	var brain cognitionpolicy.AttestedBrain
	if err := json.Unmarshal(bundle.BrainJSON, &brain); err != nil {
		panic(fmt.Sprintf("validated provider failure bootstrap changed: %v", err))
	}
	return brain
}

func decodeProviderFailureReceipt(raw []byte, target any) error {
	if err := exactjson.ValidateObject(raw, target, "provider activation failure receipt"); err != nil {
		return fmt.Errorf("%w: %v", ErrCognitionConflict, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	canonical, err := exactjson.Canonical(target)
	if err != nil || string(canonical) != string(raw) {
		return fmt.Errorf("%w: provider activation failure receipt is not canonical", ErrCognitionConflict)
	}
	return nil
}

func (r *Repository) ReadCognitionProviderActivationFailureBody(
	ctx context.Context,
	request CognitionProviderActivationFailureBodyRequest,
) (CognitionProviderIdentityEvidenceBodyPage, error) {
	if r == nil || r.pool == nil || ctx == nil || request.validate() != nil {
		return CognitionProviderIdentityEvidenceBodyPage{},
			fmt.Errorf("provider activation failure body requires PostgreSQL and a bounded scope")
	}
	columns := `operations.request_sha256,operations.request_bytes,
		substring(operations.request_body FROM $10+1 FOR $11)`
	if request.Kind == CognitionProviderIdentityResponseBody {
		columns = `operations.response_sha256,operations.response_bytes,
			substring(operations.response_body FROM $10+1 FOR $11)`
	}
	authority := request.Authority
	query := `SELECT evidence.ref_json,` + columns + `
		FROM cognition_provider_activation_failures failures
		JOIN cognition_provider_identity_evidence evidence ON evidence.evidence_id=$2
		JOIN cognition_provider_identity_evidence_operations operations
		  ON operations.evidence_id=evidence.evidence_id
		WHERE failures.record_id=$1 AND $2 IN (failures.evidence_id,failures.bootstrap_evidence_id)
		  AND failures.episode_id=$3
		  AND failures.job_id=$4 AND failures.generation=$5 AND failures.step_id=$6
		  AND failures.step_attempt=$7 AND failures.worker_id=$8
		  AND operations.operation_index=$9`
	page := CognitionProviderIdentityEvidenceBodyPage{
		OperationIndex: request.OperationIndex, Kind: request.Kind, Offset: request.Offset,
	}
	var refJSON, body []byte
	err := r.pool.QueryRow(ctx, query, request.RecordID, request.EvidenceID, request.EpisodeID,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt,
		authority.WorkerID, request.OperationIndex, request.Offset, request.Limit,
	).Scan(&refJSON, &page.SHA256, &page.TotalBytes, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return CognitionProviderIdentityEvidenceBodyPage{},
			fmt.Errorf("%w: provider activation failure body is unavailable", ErrCognitionConflict)
	}
	if err != nil {
		return CognitionProviderIdentityEvidenceBodyPage{}, err
	}
	if err := cognitionDecodeExact(refJSON, &page.Ref); err != nil {
		return CognitionProviderIdentityEvidenceBodyPage{}, err
	}
	if page.Ref.Validate() != nil || page.Ref.ID != request.EvidenceID ||
		request.Offset > page.TotalBytes || len(body) > request.Limit ||
		request.Offset+len(body) > page.TotalBytes {
		return CognitionProviderIdentityEvidenceBodyPage{},
			fmt.Errorf("%w: provider activation failure body metadata changed", ErrCognitionConflict)
	}
	page.Content = append([]byte{}, body...)
	page.NextOffset = request.Offset + len(body)
	return page, nil
}
