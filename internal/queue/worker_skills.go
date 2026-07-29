package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/specialists"
	"github.com/jackc/pgx/v5"
)

const maxWorkerSkillPageSize = 500

func (r *Repository) SyncBootstrapSkills(
	ctx context.Context,
	specs []specialists.Spec,
) ([]specialists.SkillVersion, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("sync bootstrap skills: postgres repository is not configured")
	}
	ordered := append([]specialists.Spec(nil), specs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	seen := make(map[string]struct{}, len(ordered))
	versions := make([]specialists.SkillVersion, 0, len(ordered))
	for _, spec := range ordered {
		if _, duplicate := seen[spec.ID]; duplicate {
			return nil, fmt.Errorf("sync bootstrap skills: duplicate id %q", spec.ID)
		}
		seen[spec.ID] = struct{}{}
		version, err := r.syncBootstrapSkill(ctx, spec)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func (r *Repository) syncBootstrapSkill(
	ctx context.Context,
	spec specialists.Spec,
) (specialists.SkillVersion, error) {
	hash, err := specialists.SkillContentHash(spec, specialists.SkillKindBootstrapSpecialist)
	if err != nil {
		return specialists.SkillVersion{}, fmt.Errorf("validate bootstrap skill %q: %w", spec.ID, err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return specialists.SkillVersion{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, spec.ID); err != nil {
		return specialists.SkillVersion{}, fmt.Errorf("lock bootstrap skill %s: %w", spec.ID, err)
	}

	var activeVersion int
	var activeHash, activeOrigin string
	err = tx.QueryRow(ctx, `
		SELECT version, content_sha256, origin
		FROM worker_skills
		WHERE skill_id = $1 AND status = 'active'
	`, spec.ID).Scan(&activeVersion, &activeHash, &activeOrigin)
	if err == nil && activeOrigin != string(specialists.SkillSourceBootstrap) {
		return specialists.SkillVersion{}, fmt.Errorf(
			"bootstrap skill %s conflicts with active %s skill version %d",
			spec.ID, activeOrigin, activeVersion,
		)
	}
	if err == nil && activeHash == hash {
		version, loadErr := scanWorkerSkill(tx.QueryRow(ctx, workerSkillByVersionSQL, spec.ID, activeVersion))
		if loadErr != nil {
			return specialists.SkillVersion{}, loadErr
		}
		if err := tx.Commit(ctx); err != nil {
			return specialists.SkillVersion{}, err
		}
		return version, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return specialists.SkillVersion{}, fmt.Errorf("read active bootstrap skill %s: %w", spec.ID, err)
	}

	var nextVersion int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1 FROM worker_skills WHERE skill_id = $1
	`, spec.ID).Scan(&nextVersion); err != nil {
		return specialists.SkillVersion{}, fmt.Errorf("allocate bootstrap skill %s version: %w", spec.ID, err)
	}
	if err := insertWorkerSkill(ctx, tx, specialists.SkillVersion{
		Spec: spec, Version: nextVersion, Status: specialists.SkillStatusCandidate,
		Source: specialists.SkillSourceBootstrap, Kind: specialists.SkillKindBootstrapSpecialist,
		ContentSHA256: hash,
	}); err != nil {
		return specialists.SkillVersion{}, err
	}
	if err := transitionWorkerSkill(ctx, tx, spec.ID, nextVersion,
		specialists.SkillStatusCandidate, specialists.SkillStatusValidating, nil); err != nil {
		return specialists.SkillVersion{}, err
	}
	validation := json.RawMessage(`[{"check":"bootstrap_contract","status":"passed"}]`)
	if _, err := tx.Exec(ctx, `
		INSERT INTO worker_skill_checks (skill_id, skill_version, check_name, status, detail)
		VALUES ($1, $2, 'bootstrap_contract', 'passed', 'Static bootstrap contract parsed and validated.')
	`, spec.ID, nextVersion); err != nil {
		return specialists.SkillVersion{}, fmt.Errorf("record bootstrap skill %s validation: %w", spec.ID, err)
	}
	if activeVersion > 0 {
		if err := transitionWorkerSkill(ctx, tx, spec.ID, activeVersion,
			specialists.SkillStatusActive, specialists.SkillStatusRetired, nil); err != nil {
			return specialists.SkillVersion{}, err
		}
	}
	if err := transitionWorkerSkill(ctx, tx, spec.ID, nextVersion,
		specialists.SkillStatusValidating, specialists.SkillStatusActive, validation); err != nil {
		return specialists.SkillVersion{}, err
	}
	version, err := scanWorkerSkill(tx.QueryRow(ctx, workerSkillByVersionSQL, spec.ID, nextVersion))
	if err != nil {
		return specialists.SkillVersion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return specialists.SkillVersion{}, err
	}
	return version, nil
}

func (r *Repository) ListActiveSkills(
	ctx context.Context,
	limit, offset int,
) ([]specialists.SkillVersion, error) {
	if limit < 1 || limit > maxWorkerSkillPageSize {
		return nil, fmt.Errorf("active skill page limit must be between 1 and %d", maxWorkerSkillPageSize)
	}
	if offset < 0 {
		return nil, fmt.Errorf("active skill page offset cannot be negative")
	}
	rows, err := r.pool.Query(ctx, workerSkillSelectSQL+`
		WHERE status = 'active'
		ORDER BY skill_id ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := make([]specialists.SkillVersion, 0)
	for rows.Next() {
		version, err := scanWorkerSkill(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

const workerSkillSelectSQL = `
	SELECT skill_id, version, status, origin, skill_kind, purpose, instructions,
	       preferred_models, allowed_tools, forbidden_tools, context_budget,
	       input_schema, output_schema, stop_conditions, retry_policy,
	       require_evidence, content_sha256, created_by_job_id, validation
	FROM worker_skills
`

const workerSkillByVersionSQL = workerSkillSelectSQL + `
	WHERE skill_id = $1 AND version = $2
`

type skillRow interface {
	Scan(dest ...any) error
}

func scanWorkerSkill(row skillRow, trailing ...any) (specialists.SkillVersion, error) {
	var version specialists.SkillVersion
	var status, source, kind string
	var inputSchema, outputSchema, validation []byte
	var createdByJobID *int64
	destinations := []any{
		&version.Spec.ID, &version.Version, &status, &source, &kind,
		&version.Spec.Purpose, &version.Spec.Instructions,
		&version.Spec.PreferredModel, &version.Spec.AllowedTools, &version.Spec.ForbiddenTools,
		&version.Spec.ContextBudget, &inputSchema, &outputSchema,
		&version.Spec.StopConditions, &version.Spec.RetryPolicy,
		&version.Spec.RequireEvidence, &version.ContentSHA256, &createdByJobID, &validation,
	}
	destinations = append(destinations, trailing...)
	err := row.Scan(destinations...)
	if err != nil {
		return specialists.SkillVersion{}, err
	}
	spec, err := specialists.SpecWithSchemaDocuments(version.Spec, inputSchema, outputSchema)
	if err != nil {
		return specialists.SkillVersion{}, err
	}
	version.Spec = spec
	version.Status = specialists.SkillStatus(status)
	version.Source = specialists.SkillSource(source)
	version.Kind = specialists.SkillKind(kind)
	version.CreatedByJobID = createdByJobID
	version.Validation = append(json.RawMessage(nil), validation...)
	if err := version.Validate(); err != nil {
		return specialists.SkillVersion{}, fmt.Errorf("database worker skill %s version %d: %w", spec.ID, version.Version, err)
	}
	return version, nil
}

func insertWorkerSkill(ctx context.Context, tx pgx.Tx, version specialists.SkillVersion) error {
	if err := version.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO worker_skills (
			skill_id, version, status, origin, skill_kind, purpose, instructions,
			preferred_models, allowed_tools, forbidden_tools, context_budget,
			input_schema, output_schema, stop_conditions, retry_policy,
			require_evidence, content_sha256, created_by_job_id, validation
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12::jsonb, $13::jsonb, $14, $15, $16, $17, $18, $19::jsonb
		)
	`,
		version.Spec.ID, version.Version, string(version.Status), string(version.Source), string(version.Kind),
		version.Spec.Purpose, version.Spec.Instructions, nonNilWorkerSkillStrings(version.Spec.PreferredModel),
		nonNilWorkerSkillStrings(version.Spec.AllowedTools), nonNilWorkerSkillStrings(version.Spec.ForbiddenTools), version.Spec.ContextBudget,
		nullableJSON(version.Spec.InputSchemaDocument()), nullableJSON(version.Spec.OutputSchemaDocument()),
		nonNilWorkerSkillStrings(version.Spec.StopConditions), version.Spec.RetryPolicy, version.Spec.RequireEvidence,
		version.ContentSHA256, version.CreatedByJobID, nullableJSONArray(version.Validation),
	)
	if err != nil {
		return fmt.Errorf("insert worker skill %s version %d: %w", version.Spec.ID, version.Version, err)
	}
	return nil
}

func nonNilWorkerSkillStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func transitionWorkerSkill(
	ctx context.Context,
	tx pgx.Tx,
	skillID string,
	version int,
	from, to specialists.SkillStatus,
	validation json.RawMessage,
) error {
	if err := specialists.ValidateSkillTransition(from, to); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE worker_skills
		SET status = $4,
		    validation = CASE WHEN $5::jsonb IS NULL THEN validation ELSE $5::jsonb END,
		    activated_at = CASE WHEN $4 = 'active' THEN NOW() ELSE activated_at END,
		    rejected_at = CASE WHEN $4 = 'rejected' THEN NOW() ELSE rejected_at END,
		    retired_at = CASE WHEN $4 = 'retired' THEN NOW() ELSE retired_at END
		WHERE skill_id = $1 AND version = $2 AND status = $3
	`, skillID, version, string(from), string(to), nullableJSON(validation))
	if err != nil {
		return fmt.Errorf("transition worker skill %s version %d: %w", skillID, version, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf(
			"transition worker skill %s version %d expected state %s but changed %d rows",
			skillID, version, from, result.RowsAffected(),
		)
	}
	return nil
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	return string(raw)
}

func nullableJSONArray(raw json.RawMessage) any {
	if len(raw) == 0 {
		return "[]"
	}
	return string(raw)
}
