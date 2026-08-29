package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type ObjectiveContinuityAuthority struct {
	Scope  *model.MemoryScope
	Replan *assemblyline.ObjectiveReplanAuthority
}

// ObjectiveContinuityAuthorities loads the current generation's exact replan
// feedback and the job's immutable channel/project scope. No scope is inferred
// for jobs that do not carry a channel turn binding.
func (r *Repository) ObjectiveContinuityAuthorities(
	ctx context.Context,
	job model.Job,
) (ObjectiveContinuityAuthority, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity requires PostgreSQL and context")
	}
	if job.ID < 1 || job.CurrentGeneration < 1 {
		return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity requires one positive current job generation")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	defer tx.Rollback(ctx)
	var stored model.Job
	if err := tx.QueryRow(ctx, `
		SELECT id,instruction,pipeline,metadata,current_generation
		FROM jobs WHERE id=$1
	`, job.ID).Scan(
		&stored.ID, &stored.Instruction, &stored.Pipeline, &stored.Metadata,
		&stored.CurrentGeneration,
	); err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	if stored.Instruction != job.Instruction || stored.Pipeline != job.Pipeline ||
		stored.CurrentGeneration != job.CurrentGeneration {
		return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity job authority changed before projection")
	}
	providedBinding, providedExists, err := channelBindingForJob(job)
	if err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	storedBinding, storedExists, err := channelBindingForJob(stored)
	if err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	if providedExists != storedExists || !providedBinding.equal(storedBinding) {
		return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity channel binding differs from durable job authority")
	}
	authority := ObjectiveContinuityAuthority{}
	if stored.CurrentGeneration > 1 {
		var purpose, boundaryAction, feedback, feedbackSHA string
		if err := tx.QueryRow(ctx, `
			SELECT purpose,boundary_action,feedback,feedback_sha256
			FROM job_generations
			WHERE job_id=$1 AND generation=$2
		`, stored.ID, stored.CurrentGeneration).Scan(
			&purpose, &boundaryAction, &feedback, &feedbackSHA,
		); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
		if purpose != "replan" || boundaryAction != "objective_resolve" ||
			len(feedback) > assemblyline.MaxObjectiveReplanFeedbackBytes {
			return ObjectiveContinuityAuthority{}, fmt.Errorf(
				"current objective generation has invalid or oversized exact replan feedback",
			)
		}
		digest := sha256.Sum256([]byte(feedback))
		if feedbackSHA != hex.EncodeToString(digest[:]) {
			return ObjectiveContinuityAuthority{}, fmt.Errorf("current objective generation feedback hash does not match")
		}
		authority.Replan = &assemblyline.ObjectiveReplanAuthority{
			JobID: stored.ID, Generation: stored.CurrentGeneration,
			Feedback: feedback, FeedbackSHA256: feedbackSHA,
		}
		if err := authority.ReplanContext().Validate(); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
	} else {
		var exactInitial bool
		if err := tx.QueryRow(ctx, `
			SELECT purpose='initial' AND feedback IS NULL AND feedback_sha256 IS NULL
			FROM job_generations WHERE job_id=$1 AND generation=1
		`, stored.ID).Scan(&exactInitial); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
		if !exactInitial {
			return ObjectiveContinuityAuthority{}, fmt.Errorf("initial objective generation authority is malformed")
		}
	}
	if providedExists {
		var scope model.MemoryScope
		var content string
		if err := tx.QueryRow(ctx, `
			SELECT channel.project_id,message.content
			FROM ai_channels AS channel
			JOIN ai_channel_messages AS message
			  ON message.channel_id=channel.id
			WHERE channel.id=$1 AND message.id=$2 AND message.role='user'
		`, providedBinding.ChannelID, providedBinding.UserMessageID).Scan(
			&scope.ProjectID, &content,
		); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
		scope.ChannelID = model.ChannelID(providedBinding.ChannelID)
		if err := scope.Validate(); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
		if scope.ProjectID != providedBinding.ProjectID || content != stored.Instruction {
			return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity scope does not match exact channel turn authority")
		}
		authority.Scope = &scope
	}
	if err := tx.Commit(ctx); err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	return authority, nil
}

func (authority ObjectiveContinuityAuthority) ReplanContext() assemblyline.ObjectiveContext {
	return assemblyline.ObjectiveContext{ReplanAuthority: authority.Replan}
}
