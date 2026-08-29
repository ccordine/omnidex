package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/specialists"
)

func requireFrozenActiveSkillEmbedding(version specialists.SkillVersion, count int64) error {
	if version.Status == specialists.SkillStatusActive && count != 1 {
		return fmt.Errorf(
			"active worker skill %s version %d has %d embeddings; exactly one frozen identity is required",
			version.Spec.ID, version.Version, count,
		)
	}
	return nil
}

const workerSkillSelectSQL = `
	SELECT skill_id, version, status, origin, skill_kind, purpose, instructions,
	       content_sha256, created_by_job_id
	FROM worker_skills
`

type skillRow interface {
	Scan(dest ...any) error
}

func scanWorkerSkill(row skillRow, trailing ...any) (specialists.SkillVersion, error) {
	var version specialists.SkillVersion
	var status, source, kind string
	var createdByJobID *int64
	destinations := []any{
		&version.Spec.ID, &version.Version, &status, &source, &kind,
		&version.Spec.Purpose, &version.Spec.Instructions,
		&version.ContentSHA256, &createdByJobID,
	}
	destinations = append(destinations, trailing...)
	err := row.Scan(destinations...)
	if err != nil {
		return specialists.SkillVersion{}, err
	}
	version.Status = specialists.SkillStatus(status)
	version.Source = specialists.SkillSource(source)
	version.Kind = specialists.SkillKind(kind)
	version.CreatedByJobID = createdByJobID
	if err := version.Validate(); err != nil {
		return specialists.SkillVersion{}, fmt.Errorf(
			"database worker skill %s version %d: %w",
			version.Spec.ID, version.Version, err,
		)
	}
	return version, nil
}
