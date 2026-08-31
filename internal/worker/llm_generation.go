package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type exactStationCall struct {
	WorkID          string
	WorkKind        assemblyline.WorkKind
	Prompt          string
	ContextTokens   int
	MaxOutputTokens int
}

type exactStationExecution struct {
	WorkID                  string
	Candidate               string
	CandidateResponseSHA256 string
}

func (s *Service) executeExactPortableStation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
) (assemblyline.PortableResult, exactStationExecution, error) {
	if ctx == nil || s == nil || s.repo == nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("exact station requires context, worker, and PostgreSQL authority")
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	if modelName == "" {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("exact station model is required")
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	contextTokens, err := s.exactStationContextTokens(ctx)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"resolve exact station context: %w", err,
		)
	}
	maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(job, contextTokens)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"derive exact station output ceiling: %w", err,
		)
	}
	call := exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Prompt: prompt,
		ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens,
	}
	if nilWorkerTransport(s.stationClient) {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station generation provider is not configured",
		)
	}
	prepared, err := prepareExactStationCall(call, modelName, nil)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	return s.dispatchExactStationCall(ctx, authority, call, prepared)
}
