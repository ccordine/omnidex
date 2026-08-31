package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type exactStationCall struct {
	WorkID          string
	WorkKind        assemblyline.WorkKind
	Prompt          string
	ContextTokens   int
	MaxOutputTokens int
	OutputLimitMode llm.ExactPreparedOutputLimitMode
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
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("exact station model is required")
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	contract, err := llmResponseContractForPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	contextTokens, err := s.exactStationContextTokens(ctx, job, modelName)
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
	if err := validateExactStationStaticCall(prompt, contract, contextTokens); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	call := exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Prompt: prompt,
		ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens,
		OutputLimitMode: contract.OutputLimitMode,
	}
	if nilWorkerTransport(s.stationClient) {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station generation provider is not configured",
		)
	}
	if err := s.stationClient.RequireExactPreparedContract(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station provider: %w", err,
		)
	}
	prepared, err := prepareExactStationCall(call, contract, modelName, nil)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	return s.dispatchExactStationCall(ctx, authority, call, prepared)
}
