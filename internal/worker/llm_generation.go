package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type exactStationExecution struct {
	Gap                     queue.StationGapOpening
	Candidate               string
	CallReceiptSHA256       string
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
	stationID, err := queue.StationForPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
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
	gap, err := s.repo.OpenStationGap(ctx, queue.StationGapOpenRecord{
		Authority: authority, Job: job, Station: stationID,
		ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens,
		OutputLimitMode: contract.OutputLimitMode,
	})
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("persist typed station gap: %w", err)
	}
	if nilWorkerTransport(s.stationClient) {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("exact station generation provider is not configured"),
		)
	}
	if err := s.stationClient.RequireExactPreparedContract(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("exact station provider: %w", err),
		)
	}
	prepared, err := prepareExactStationCall(gap, contract, modelName, nil)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, err,
		)
	}
	return s.dispatchExactStationCall(ctx, authority, gap, modelName, prepared)
}
