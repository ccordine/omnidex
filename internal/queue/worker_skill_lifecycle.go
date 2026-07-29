package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/specialists"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateLearnedSkillCandidate(
	ctx context.Context,
	spec specialists.Spec,
	createdByJobID int64,
) (specialists.SkillVersion, bool, error) {
	if createdByJobID < 1 {
		return specialists.SkillVersion{}, false, fmt.Errorf("learned skill requires a creating job")
	}
	hash, err := specialists.SkillContentHash(spec, specialists.SkillKindCodeProcedure)
	if err != nil {
		return specialists.SkillVersion{}, false, err
	}
	candidate := specialists.SkillVersion{
		Spec: spec, Version: 1, Status: specialists.SkillStatusCandidate,
		Source: specialists.SkillSourceLearned, Kind: specialists.SkillKindCodeProcedure,
		CreatedByJobID: &createdByJobID,
		ContentSHA256:  hash,
	}
	if err := candidate.Validate(); err != nil {
		return specialists.SkillVersion{}, false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return specialists.SkillVersion{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, spec.ID); err != nil {
		return specialists.SkillVersion{}, false, err
	}

	var matchingVersion int
	var matchingStatus string
	var matchingJobID *int64
	err = tx.QueryRow(ctx, `
		SELECT version, status, created_by_job_id
		FROM worker_skills
		WHERE skill_id = $1 AND content_sha256 = $2
		ORDER BY version DESC
		LIMIT 1
	`, spec.ID, hash).Scan(&matchingVersion, &matchingStatus, &matchingJobID)
	if err == nil {
		stored, loadErr := scanWorkerSkill(tx.QueryRow(ctx, workerSkillByVersionSQL, spec.ID, matchingVersion))
		if loadErr != nil {
			return specialists.SkillVersion{}, false, loadErr
		}
		switch stored.Status {
		case specialists.SkillStatusActive:
			if err := tx.Commit(ctx); err != nil {
				return specialists.SkillVersion{}, false, err
			}
			return stored, false, nil
		case specialists.SkillStatusCandidate, specialists.SkillStatusValidating:
			if matchingJobID == nil || *matchingJobID != createdByJobID {
				return specialists.SkillVersion{}, false, fmt.Errorf(
					"learned skill %s has an unfinished version owned by another job", spec.ID,
				)
			}
			if err := tx.Commit(ctx); err != nil {
				return specialists.SkillVersion{}, false, err
			}
			return stored, false, nil
		case specialists.SkillStatusRejected:
			return specialists.SkillVersion{}, false, fmt.Errorf(
				"learned skill %s repeats rejected content without a correction", spec.ID,
			)
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return specialists.SkillVersion{}, false, err
	}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1 FROM worker_skills WHERE skill_id = $1
	`, spec.ID).Scan(&candidate.Version); err != nil {
		return specialists.SkillVersion{}, false, err
	}
	if err := insertWorkerSkill(ctx, tx, candidate); err != nil {
		return specialists.SkillVersion{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return specialists.SkillVersion{}, false, err
	}
	return candidate, true, nil
}

func (r *Repository) BeginWorkerSkillValidation(
	ctx context.Context,
	skillID string,
	version int,
	check specialists.SkillCheck,
) error {
	if check.Status != specialists.SkillCheckPassed {
		return fmt.Errorf("initial worker skill validation check must pass")
	}
	if err := check.Validate(); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := transitionWorkerSkill(ctx, tx, skillID, version,
		specialists.SkillStatusCandidate, specialists.SkillStatusValidating, nil); err != nil {
		return err
	}
	if err := insertWorkerSkillCheck(ctx, tx, skillID, version, check); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) RecordWorkerSkillCheck(
	ctx context.Context,
	skillID string,
	version int,
	check specialists.SkillCheck,
) error {
	if err := check.Validate(); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM worker_skills
		WHERE skill_id=$1 AND version=$2
		FOR UPDATE
	`, skillID, version).Scan(&status); err != nil {
		return err
	}
	if specialists.SkillStatus(status) != specialists.SkillStatusValidating {
		return fmt.Errorf("worker skill %s version %d is %s, not validating", skillID, version, status)
	}
	if err := insertWorkerSkillCheck(ctx, tx, skillID, version, check); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ActivateWorkerSkill(ctx context.Context, skillID string, version int) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, skillID); err != nil {
		return err
	}
	if err := requireLearnedSkillActivationInputs(ctx, tx, skillID, version); err != nil {
		return err
	}
	evidence, err := workerSkillValidationEvidence(ctx, tx, skillID, version, false)
	if err != nil {
		return err
	}
	if err := requirePassedWorkerSkillChecks(evidence,
		"contract", "isolated_stage", "workspace_verification",
	); err != nil {
		return fmt.Errorf("activate worker skill %s version %d: %w", skillID, version, err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE worker_skills
		SET status='retired', retired_at=NOW()
		WHERE skill_id=$1 AND status='active' AND version<>$2
	`, skillID, version); err != nil {
		return err
	}
	if err := transitionWorkerSkill(ctx, tx, skillID, version,
		specialists.SkillStatusValidating, specialists.SkillStatusActive, evidence); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) RejectWorkerSkill(
	ctx context.Context,
	skillID string,
	version int,
	check specialists.SkillCheck,
) error {
	if check.Status != specialists.SkillCheckFailed {
		return fmt.Errorf("worker skill rejection requires one failed check")
	}
	if err := check.Validate(); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var rawStatus string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM worker_skills
		WHERE skill_id=$1 AND version=$2
		FOR UPDATE
	`, skillID, version).Scan(&rawStatus); err != nil {
		return err
	}
	status := specialists.SkillStatus(rawStatus)
	if status != specialists.SkillStatusCandidate && status != specialists.SkillStatusValidating {
		return fmt.Errorf("worker skill %s version %d cannot be rejected from %s", skillID, version, status)
	}
	if err := insertWorkerSkillCheck(ctx, tx, skillID, version, check); err != nil {
		return err
	}
	evidence, err := workerSkillValidationEvidence(ctx, tx, skillID, version, true)
	if err != nil {
		return err
	}
	if err := transitionWorkerSkill(ctx, tx, skillID, version, status,
		specialists.SkillStatusRejected, evidence); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
