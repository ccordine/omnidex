package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
)

func (r *Repository) SaveDataSourceCatalogByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	catalog datasource.SchemaCatalog,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "save data source catalog", func() error {
		return r.SaveDataSourceCatalog(ctx, catalog)
	})
}

func (r *Repository) UpdateDataSourceCatalogTimestampByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	id string,
	at time.Time,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "update data source catalog timestamp", func() error {
		return r.UpdateDataSourceCatalogTimestamp(ctx, id, at)
	})
}

func (r *Repository) AddDataSourceChannelMessageByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	channelID, role, content string,
	payload json.RawMessage,
	jobID *int64,
) (model.DataSourceChannelMessage, error) {
	if jobID == nil || *jobID != authority.JobID {
		return model.DataSourceChannelMessage{}, fmt.Errorf(
			"%w: data source result job disagrees with step attempt", ErrStaleStepAttempt,
		)
	}
	return underActiveStepAttemptFence(ctx, r, authority, "append data source channel result", func() (model.DataSourceChannelMessage, error) {
		return r.AddDataSourceChannelMessage(ctx, channelID, role, content, payload, jobID)
	})
}

func (r *Repository) AddMemoryChunkByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	source, kind, content string,
	tags []string,
	embedding []float64,
) (model.MemoryChunk, error) {
	return underActiveStepAttemptFence(ctx, r, authority, "add durable memory", func() (model.MemoryChunk, error) {
		return r.AddMemoryChunk(ctx, source, kind, content, tags, embedding)
	})
}

func (r *Repository) CreateProjectDebuggerCardJobByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	projectID int64,
	input ProjectDebuggerCardInput,
) (DBScrumCard, model.Job, error) {
	type result struct {
		card DBScrumCard
		job  model.Job
	}
	created, err := underActiveStepAttemptFence(ctx, r, authority, "create project debugger card", func() (result, error) {
		card, job, err := r.CreateProjectDebuggerCardJob(ctx, projectID, input)
		return result{card: card, job: job}, err
	})
	return created.card, created.job, err
}

func (r *Repository) UpdateProjectSettingByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	projectID int64,
	key string,
	value json.RawMessage,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "update project setting", func() error {
		return r.UpdateProjectSetting(ctx, projectID, key, value)
	})
}

func (r *Repository) UpdateScrumCardByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	projectID int64,
	cardID string,
	patch map[string]any,
) (DBScrumCard, error) {
	return underActiveStepAttemptFence(ctx, r, authority, "update Scrum card", func() (DBScrumCard, error) {
		return r.UpdateScrumCard(ctx, projectID, cardID, patch)
	})
}
