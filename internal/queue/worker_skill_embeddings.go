package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/specialists"
	"github.com/jackc/pgx/v5"
)

const (
	maxWorkerSkillMatches     = 5
	maxWorkerSkillVectorItems = 16_000
)

type WorkerSkillMatch struct {
	Version  specialists.SkillVersion
	Distance float64
}

func (r *Repository) ActiveLearnedSkill(
	ctx context.Context,
	skillID string,
) (specialists.SkillVersion, bool, error) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return specialists.SkillVersion{}, false, fmt.Errorf("active learned skill requires one id")
	}
	if r == nil || r.pool == nil {
		return specialists.SkillVersion{}, false, fmt.Errorf("active learned skill requires PostgreSQL")
	}
	version, err := scanWorkerSkill(r.pool.QueryRow(ctx, workerSkillSelectSQL+`
		WHERE skill_id=$1 AND status='active' AND origin='learned' AND skill_kind='code_procedure'
	`, skillID))
	if errors.Is(err, pgx.ErrNoRows) {
		return specialists.SkillVersion{}, false, nil
	}
	if err != nil {
		return specialists.SkillVersion{}, false, err
	}
	return version, true, nil
}

func (r *Repository) StoreWorkerSkillEmbedding(
	ctx context.Context,
	skillID string,
	version int,
	provider string,
	modelName string,
	embedding []float64,
) error {
	skillID, provider, modelName, literal, digest, err := validatedWorkerSkillEmbedding(
		skillID, provider, modelName, embedding,
	)
	if err != nil {
		return err
	}
	if version < 1 {
		return fmt.Errorf("worker skill embedding version must be positive")
	}
	if r == nil || r.pool == nil {
		return fmt.Errorf("worker skill embedding requires PostgreSQL")
	}
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO worker_skill_embeddings (
			skill_id, skill_version, embedding_provider, embedding_model,
			embedding, embedding_sha256
		)
		SELECT skill_id, version, $3, $4, $5::vector, $6
		FROM worker_skills
		WHERE skill_id=$1 AND version=$2
		  AND origin='learned' AND skill_kind='code_procedure'
		  AND status IN ('candidate', 'validating', 'active')
		ON CONFLICT DO NOTHING
	`, skillID, version, provider, modelName, literal, digest)
	if err != nil {
		return fmt.Errorf("store worker skill %s version %d embedding: %w", skillID, version, err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var storedHash string
	err = r.pool.QueryRow(ctx, `
		SELECT embedding_sha256
		FROM worker_skill_embeddings
		WHERE skill_id=$1 AND skill_version=$2
		  AND embedding_provider=$3 AND embedding_model=$4
	`, skillID, version, provider, modelName).Scan(&storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("worker skill %s version %d is missing or cannot receive an embedding", skillID, version)
	}
	if err != nil {
		return err
	}
	if storedHash != digest {
		return fmt.Errorf(
			"worker skill %s version %d already has a different immutable embedding for %s/%s",
			skillID, version, provider, modelName,
		)
	}
	return nil
}

func (r *Repository) FindActiveWorkerSkillMatches(
	ctx context.Context,
	provider string,
	modelName string,
	embedding []float64,
	limit int,
) ([]WorkerSkillMatch, error) {
	if limit < 1 || limit > maxWorkerSkillMatches {
		return nil, fmt.Errorf("worker skill match limit must be between 1 and %d", maxWorkerSkillMatches)
	}
	_, provider, modelName, literal, _, err := validatedWorkerSkillEmbedding(
		"query", provider, modelName, embedding,
	)
	if err != nil {
		return nil, err
	}
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("worker skill matching requires PostgreSQL")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT skills.*, embeddings.embedding <=> $3::vector AS distance
		FROM (`+workerSkillSelectSQL+`) AS skills
		JOIN worker_skill_embeddings AS embeddings
		  ON embeddings.skill_id=skills.skill_id AND embeddings.skill_version=skills.version
		WHERE skills.status='active' AND skills.origin='learned'
		  AND skills.skill_kind='code_procedure'
		  AND embeddings.embedding_provider=$1 AND embeddings.embedding_model=$2
		ORDER BY embeddings.embedding <=> $3::vector ASC, skills.skill_id ASC
		LIMIT $4
	`, provider, modelName, literal, limit)
	if err != nil {
		return nil, fmt.Errorf("find active worker skill matches: %w", err)
	}
	defer rows.Close()
	matches := make([]WorkerSkillMatch, 0, limit)
	for rows.Next() {
		var distance float64
		version, err := scanWorkerSkill(rows, &distance)
		if err != nil {
			return nil, err
		}
		if math.IsNaN(distance) || math.IsInf(distance, 0) || distance < 0 {
			return nil, fmt.Errorf("worker skill %s returned invalid cosine distance", version.Spec.ID)
		}
		matches = append(matches, WorkerSkillMatch{Version: version, Distance: distance})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return matches, nil
}

func validatedWorkerSkillEmbedding(
	skillID string,
	provider string,
	modelName string,
	embedding []float64,
) (string, string, string, string, string, error) {
	skillID = strings.TrimSpace(skillID)
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if skillID == "" || provider == "" || modelName == "" {
		return "", "", "", "", "", fmt.Errorf("worker skill embedding requires id, provider, and model")
	}
	if len(provider) > 128 || len(modelName) > 256 {
		return "", "", "", "", "", fmt.Errorf("worker skill embedding identity exceeds its hard limit")
	}
	if len(embedding) < 1 || len(embedding) > maxWorkerSkillVectorItems {
		return "", "", "", "", "", fmt.Errorf(
			"worker skill embedding dimensions must be between 1 and %d", maxWorkerSkillVectorItems,
		)
	}
	parts := make([]string, len(embedding))
	for index, value := range embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", "", "", "", "", fmt.Errorf("worker skill embedding contains a non-finite value at %d", index)
		}
		parts[index] = strconv.FormatFloat(value, 'g', -1, 64)
	}
	literal := "[" + strings.Join(parts, ",") + "]"
	hash := sha256.Sum256([]byte(literal))
	return skillID, provider, modelName, literal, hex.EncodeToString(hash[:]), nil
}
