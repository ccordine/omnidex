package queue

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	maxClaimsPerWrite      = 128
	maxClaimTextBytes      = 64 << 10
	maxClaimRationaleBytes = 16 << 10
	claimStatusSupported   = "supported"
	claimStatusUnsupported = "unsupported"
)

func (r *Repository) WriteClaims(ctx context.Context, claims []model.ClaimRecord) ([]model.ClaimRecord, error) {
	if len(claims) == 0 {
		return []model.ClaimRecord{}, nil
	}
	if err := validateClaimBatch(claims); err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	jobID, stepID := claims[0].JobID, claims[0].StepID
	if err := requireRunningCurrentStepTx(ctx, tx, jobID, stepID); err != nil {
		return nil, err
	}
	saved := make([]model.ClaimRecord, 0, len(claims))
	for _, claim := range claims {
		if err := tx.QueryRow(ctx, `
			INSERT INTO claims (job_id, step_id, text, normalized_text, status, confidence)
			SELECT steps.job_id, steps.id, $3, $4, $5, $6
			FROM job_steps AS steps
			JOIN jobs ON jobs.id = steps.job_id
			WHERE steps.job_id = $1
			  AND steps.id = $2
			  AND steps.superseded_at_generation IS NULL
			  AND steps.generation = jobs.current_generation
			RETURNING id, created_at
		`, jobID, stepID, claim.Text, claim.NormalizedText, claim.Status, claim.Confidence).Scan(
			&claim.ID, &claim.CreatedAt,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("%w: claim step %d is no longer current", ErrStaleJobGeneration, stepID)
			}
			return nil, err
		}
		saved = append(saved, claim)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return saved, nil
}

func (r *Repository) WriteClaimSupports(ctx context.Context, supports []model.ClaimSupportRecord) error {
	if len(supports) == 0 {
		return nil
	}
	if err := validateClaimSupports(supports); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	jobID, err := claimSupportBatchJob(ctx, tx, supports)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM jobs WHERE id = $1 FOR UPDATE`, jobID); err != nil {
		return err
	}
	for _, support := range supports {
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO claim_support (
				job_id, claim_id, evidence_id, support_score, rationale
			)
			SELECT claims.job_id, claims.id, evidence.id, $3, $4
			FROM claims
			JOIN job_steps AS claim_steps ON claim_steps.id = claims.step_id
			JOIN jobs ON jobs.id = claims.job_id
			JOIN evidence ON evidence.id = $2 AND evidence.job_id = claims.job_id
			JOIN job_steps AS evidence_steps ON evidence_steps.id = evidence.step_id
			WHERE claims.id = $1
			  AND claim_steps.superseded_at_generation IS NULL
			  AND claim_steps.generation = jobs.current_generation
			  AND claim_steps.status = $5
			  AND evidence_steps.superseded_at_generation IS NULL
			RETURNING claim_support.id
		`, support.ClaimID, support.EvidenceID, support.SupportScore, support.Rationale,
			model.StepStatusRunning).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf(
					"%w: claim %d or evidence %d is not current for job %d",
					ErrStaleJobGeneration, support.ClaimID, support.EvidenceID, jobID,
				)
			}
			return err
		}
		support.ID = id
	}
	return tx.Commit(ctx)
}

func claimSupportBatchJob(ctx context.Context, tx pgx.Tx, supports []model.ClaimSupportRecord) (int64, error) {
	var jobID int64
	for index, support := range supports {
		var claimJobID, evidenceJobID int64
		if err := tx.QueryRow(ctx, `
			SELECT claims.job_id, evidence.job_id
			FROM claims
			JOIN evidence ON evidence.id = $2
			WHERE claims.id = $1
		`, support.ClaimID, support.EvidenceID).Scan(&claimJobID, &evidenceJobID); err != nil {
			return 0, err
		}
		if claimJobID != evidenceJobID {
			return 0, fmt.Errorf(
				"claim %d belongs to job %d but evidence %d belongs to job %d",
				support.ClaimID, claimJobID, support.EvidenceID, evidenceJobID,
			)
		}
		if index == 0 {
			jobID = claimJobID
		}
		if claimJobID != jobID {
			return 0, fmt.Errorf("claim support batch spans jobs %d and %d", jobID, claimJobID)
		}
	}
	return jobID, nil
}

func validateClaimBatch(claims []model.ClaimRecord) error {
	if len(claims) > maxClaimsPerWrite {
		return fmt.Errorf("claim batch has %d records; maximum is %d", len(claims), maxClaimsPerWrite)
	}
	jobID, stepID := claims[0].JobID, claims[0].StepID
	if jobID <= 0 || stepID <= 0 {
		return errors.New("claim job and step identities are required")
	}
	for index, claim := range claims {
		if claim.JobID != jobID || claim.StepID != stepID {
			return fmt.Errorf("claim %d does not share batch job and step authority", index)
		}
		if err := validateExactClaimText("claim text", claim.Text, maxClaimTextBytes); err != nil {
			return fmt.Errorf("claim %d: %w", index, err)
		}
		if err := validateExactClaimText("normalized claim text", claim.NormalizedText, maxClaimTextBytes); err != nil {
			return fmt.Errorf("claim %d: %w", index, err)
		}
		if claim.Status != claimStatusSupported && claim.Status != claimStatusUnsupported {
			return fmt.Errorf("claim %d has unregistered status %q", index, claim.Status)
		}
		if math.IsNaN(claim.Confidence) || math.IsInf(claim.Confidence, 0) || claim.Confidence < 0 || claim.Confidence > 1 {
			return fmt.Errorf("claim %d confidence must be between zero and one", index)
		}
	}
	return nil
}

func validateClaimSupports(supports []model.ClaimSupportRecord) error {
	if len(supports) > maxClaimsPerWrite {
		return fmt.Errorf("claim support batch has %d records; maximum is %d", len(supports), maxClaimsPerWrite)
	}
	seen := make(map[[2]int64]struct{}, len(supports))
	for index, support := range supports {
		if support.ClaimID <= 0 || support.EvidenceID <= 0 {
			return fmt.Errorf("claim support %d requires claim and evidence identities", index)
		}
		identity := [2]int64{support.ClaimID, support.EvidenceID}
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("claim support %d duplicates claim %d evidence %d", index, support.ClaimID, support.EvidenceID)
		}
		seen[identity] = struct{}{}
		if math.IsNaN(support.SupportScore) || math.IsInf(support.SupportScore, 0) || support.SupportScore < 0 || support.SupportScore > 1 {
			return fmt.Errorf("claim support %d score must be between zero and one", index)
		}
		if err := validateExactClaimText("claim support rationale", support.Rationale, maxClaimRationaleBytes); err != nil {
			return fmt.Errorf("claim support %d: %w", index, err)
		}
	}
	return nil
}

func validateExactClaimText(field, value string, maxBytes int) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must be nonempty exact trimmed text", field)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must be PostgreSQL-compatible UTF-8", field)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%s has %d bytes; maximum is %d", field, len(value), maxBytes)
	}
	return nil
}
