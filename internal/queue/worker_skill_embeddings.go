package queue

import (
	"context"
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
	var embeddingCount int64
	version, err := scanWorkerSkill(r.pool.QueryRow(ctx, `
		SELECT skills.*, (
			SELECT COUNT(*) FROM worker_skill_embeddings AS embeddings
			WHERE embeddings.skill_id=skills.skill_id
			  AND embeddings.skill_version=skills.version
		) AS embedding_count
		FROM (`+workerSkillSelectSQL+`) AS skills
		WHERE skills.skill_id=$1 AND skills.status='active'
		  AND skills.origin='learned' AND skills.skill_kind='code_procedure'
	`, skillID), &embeddingCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return specialists.SkillVersion{}, false, nil
	}
	if err != nil {
		return specialists.SkillVersion{}, false, err
	}
	if err := requireFrozenActiveSkillEmbedding(version, embeddingCount); err != nil {
		return specialists.SkillVersion{}, false, err
	}
	return version, true, nil
}

func (r *Repository) HasActiveWorkerSkillEmbeddings(
	ctx context.Context,
	provider string,
	modelName string,
) (bool, error) {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" || modelName == "" {
		return false, fmt.Errorf("active worker skill lookup requires provider and model")
	}
	if len(provider) > 128 || len(modelName) > 256 {
		return false, fmt.Errorf("active worker skill embedding identity exceeds its hard limit")
	}
	if r == nil || r.pool == nil {
		return false, fmt.Errorf("active worker skill lookup requires PostgreSQL")
	}
	var invalid, exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM worker_skills AS skills
			LEFT JOIN worker_skill_embeddings AS embeddings
			  ON embeddings.skill_id=skills.skill_id
			 AND embeddings.skill_version=skills.version
			WHERE skills.status='active' AND skills.origin='learned'
			  AND skills.skill_kind='code_procedure'
			GROUP BY skills.skill_id,skills.version
			HAVING COUNT(embeddings.skill_id)<>1
		), EXISTS (
			SELECT 1
			FROM worker_skills AS skills
			JOIN worker_skill_embeddings AS embeddings
			  ON embeddings.skill_id=skills.skill_id
			 AND embeddings.skill_version=skills.version
			WHERE skills.status='active' AND skills.origin='learned'
			  AND skills.skill_kind='code_procedure'
			  AND embeddings.embedding_provider=$1
			  AND embeddings.embedding_model=$2
		)
	`, provider, modelName).Scan(&invalid, &exists)
	if err != nil {
		return false, fmt.Errorf("check active worker skill embeddings: %w", err)
	}
	if invalid {
		return false, fmt.Errorf("active worker skill registry contains a version without exactly one frozen embedding identity")
	}
	return exists, nil
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
	provider, modelName, literal, err := validatedWorkerSkillQuery(provider, modelName, embedding)
	if err != nil {
		return nil, err
	}
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("worker skill matching requires PostgreSQL")
	}
	hasActive, err := r.HasActiveWorkerSkillEmbeddings(ctx, provider, modelName)
	if err != nil {
		return nil, err
	}
	if !hasActive {
		return []WorkerSkillMatch{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT skills.*, embeddings.embedding <=> $3::vector AS distance, (
			SELECT COUNT(*) FROM worker_skill_embeddings AS frozen
			WHERE frozen.skill_id=skills.skill_id
			  AND frozen.skill_version=skills.version
		) AS embedding_count
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
		var embeddingCount int64
		version, err := scanWorkerSkill(rows, &distance, &embeddingCount)
		if err != nil {
			return nil, err
		}
		if err := requireFrozenActiveSkillEmbedding(version, embeddingCount); err != nil {
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

func validatedWorkerSkillQuery(
	provider string,
	modelName string,
	embedding []float64,
) (string, string, string, error) {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" || modelName == "" {
		return "", "", "", fmt.Errorf("worker skill query requires provider and model")
	}
	if len(provider) > 128 || len(modelName) > 256 {
		return "", "", "", fmt.Errorf("worker skill embedding identity exceeds its hard limit")
	}
	if len(embedding) < 1 || len(embedding) > maxWorkerSkillVectorItems {
		return "", "", "", fmt.Errorf(
			"worker skill embedding dimensions must be between 1 and %d", maxWorkerSkillVectorItems,
		)
	}
	parts := make([]string, len(embedding))
	for index, value := range embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", "", "", fmt.Errorf("worker skill embedding contains a non-finite value at %d", index)
		}
		parts[index] = strconv.FormatFloat(value, 'g', -1, 64)
	}
	literal := "[" + strings.Join(parts, ",") + "]"
	return provider, modelName, literal, nil
}
