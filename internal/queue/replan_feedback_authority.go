package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func (r *Repository) CurrentReplanAuthority(
	ctx context.Context,
	job model.Job,
	expectedBoundary string,
) (*assemblyline.ObjectiveReplanAuthority, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("replan feedback requires PostgreSQL and context")
	}
	if job.ID < 1 || job.CurrentGeneration < 2 {
		return nil, fmt.Errorf("replan feedback requires a current generation after the initial generation")
	}
	switch expectedBoundary {
	case replanCodingBoundary, replanObjectiveBoundary:
	default:
		return nil, fmt.Errorf("replan feedback boundary %q is not registered", expectedBoundary)
	}
	var instruction, pipeline, purpose, boundary, feedback, feedbackSHA string
	var generation int64
	err := r.pool.QueryRow(ctx, `
		SELECT jobs.instruction,jobs.pipeline,jobs.current_generation,
		       generation.purpose,generation.boundary_action,
		       generation.feedback,generation.feedback_sha256
		FROM jobs
		JOIN job_generations AS generation
		  ON generation.job_id=jobs.id AND generation.generation=jobs.current_generation
		WHERE jobs.id=$1
	`, job.ID).Scan(
		&instruction, &pipeline, &generation, &purpose, &boundary, &feedback, &feedbackSHA,
	)
	if err != nil {
		return nil, err
	}
	if instruction != job.Instruction || pipeline != job.Pipeline || generation != job.CurrentGeneration {
		return nil, fmt.Errorf("replan feedback job authority changed before consumption")
	}
	if purpose != jobGenerationPurposeReplan || boundary != expectedBoundary {
		return nil, fmt.Errorf("current generation is not an exact %s replan", expectedBoundary)
	}
	authority := &assemblyline.ObjectiveReplanAuthority{
		JobID: job.ID, Generation: generation, Feedback: feedback, FeedbackSHA256: feedbackSHA,
	}
	if err := (assemblyline.ObjectiveContext{ReplanAuthority: authority}).Validate(); err != nil {
		return nil, err
	}
	return authority, nil
}
