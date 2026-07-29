package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/specialists"
	"github.com/jackc/pgx/v5"
)

func insertWorkerSkillCheck(
	ctx context.Context,
	tx pgx.Tx,
	skillID string,
	version int,
	check specialists.SkillCheck,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO worker_skill_checks (skill_id, skill_version, check_name, status, detail)
		VALUES ($1, $2, $3, $4, $5)
	`, skillID, version, check.Name, string(check.Status), check.Detail)
	if err != nil {
		return fmt.Errorf("record worker skill %s version %d check %s: %w", skillID, version, check.Name, err)
	}
	return nil
}

func workerSkillValidationEvidence(
	ctx context.Context,
	tx pgx.Tx,
	skillID string,
	version int,
	allowFailed bool,
) (json.RawMessage, error) {
	rows, err := tx.Query(ctx, `
		SELECT check_name, status, detail
		FROM worker_skill_checks
		WHERE skill_id=$1 AND skill_version=$2
		ORDER BY id ASC
	`, skillID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	checks := make([]specialists.SkillCheck, 0)
	failed := false
	for rows.Next() {
		var check specialists.SkillCheck
		var status string
		if err := rows.Scan(&check.Name, &status, &check.Detail); err != nil {
			return nil, err
		}
		check.Status = specialists.SkillCheckStatus(status)
		if err := check.Validate(); err != nil {
			return nil, err
		}
		failed = failed || check.Status == specialists.SkillCheckFailed
		checks = append(checks, check)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(checks) == 0 {
		return nil, fmt.Errorf("worker skill %s version %d has no validation evidence", skillID, version)
	}
	if failed && !allowFailed {
		return nil, fmt.Errorf("worker skill %s version %d has failed validation evidence", skillID, version)
	}
	raw, err := json.Marshal(checks)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func requirePassedWorkerSkillChecks(raw json.RawMessage, names ...string) error {
	var checks []specialists.SkillCheck
	if err := json.Unmarshal(raw, &checks); err != nil {
		return fmt.Errorf("decode worker skill validation evidence: %w", err)
	}
	passed := make(map[string]bool, len(checks))
	for _, check := range checks {
		if err := check.Validate(); err != nil {
			return err
		}
		passed[check.Name] = check.Status == specialists.SkillCheckPassed
	}
	for _, name := range names {
		if !passed[name] {
			return fmt.Errorf("required validation check %q has not passed", name)
		}
	}
	return nil
}

func requireLearnedSkillActivationInputs(
	ctx context.Context,
	tx pgx.Tx,
	skillID string,
	version int,
) error {
	var status, origin, kind string
	var hasEmbedding bool
	if err := tx.QueryRow(ctx, `
		SELECT status, origin, skill_kind,
		       EXISTS (
		           SELECT 1 FROM worker_skill_embeddings
		           WHERE skill_id=$1 AND skill_version=$2
		       )
		FROM worker_skills
		WHERE skill_id=$1 AND version=$2
		FOR UPDATE
	`, skillID, version).Scan(&status, &origin, &kind, &hasEmbedding); err != nil {
		return err
	}
	if status != string(specialists.SkillStatusValidating) ||
		origin != string(specialists.SkillSourceLearned) ||
		kind != string(specialists.SkillKindCodeProcedure) {
		return fmt.Errorf(
			"worker skill %s version %d is not a validating learned code procedure",
			skillID, version,
		)
	}
	if !hasEmbedding {
		return fmt.Errorf("worker skill %s version %d has no semantic retrieval embedding", skillID, version)
	}
	return nil
}
