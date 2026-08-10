package queue

import (
	"context"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialists"
)

func (r *Repository) CreateLearnedSkillCandidateByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	spec specialists.Spec,
) (specialists.SkillVersion, bool, error) {
	type result struct {
		version specialists.SkillVersion
		created bool
	}
	stored, err := underActiveStepAttemptFence(ctx, r, authority, "create learned skill candidate", func() (result, error) {
		version, created, err := r.CreateLearnedSkillCandidate(ctx, spec, authority.JobID)
		return result{version: version, created: created}, err
	})
	return stored.version, stored.created, err
}

func (r *Repository) StoreWorkerSkillEmbeddingByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	skillID string,
	version int,
	provider, modelName string,
	embedding []float64,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "store learned skill embedding", func() error {
		return r.StoreWorkerSkillEmbedding(ctx, skillID, version, provider, modelName, embedding)
	})
}

func (r *Repository) BeginWorkerSkillValidationByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	skillID string,
	version int,
	check specialists.SkillCheck,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "begin learned skill validation", func() error {
		return r.BeginWorkerSkillValidation(ctx, skillID, version, check)
	})
}

func (r *Repository) RecordWorkerSkillCheckByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	skillID string,
	version int,
	check specialists.SkillCheck,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "record learned skill check", func() error {
		return r.RecordWorkerSkillCheck(ctx, skillID, version, check)
	})
}

func (r *Repository) ActivateWorkerSkillByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	skillID string,
	version int,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "activate learned skill", func() error {
		return r.ActivateWorkerSkill(ctx, skillID, version)
	})
}

func (r *Repository) RejectWorkerSkillByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	skillID string,
	version int,
	check specialists.SkillCheck,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "reject learned skill", func() error {
		return r.RejectWorkerSkill(ctx, skillID, version, check)
	})
}
