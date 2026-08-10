package cognitiongauntlet

import (
	"context"
	"fmt"
	"reflect"
)

type FullCognitionTransferResult struct {
	Authority TransferAuthority        `json:"authority"`
	Runs      []FullCognitionRunResult `json:"runs"`
	Report    TransferRailReport       `json:"report"`
}

func RunFullCognitionTransfer(
	ctx context.Context,
	fixture MicrogauntletCase,
	surfaces []Surface,
	requests []FullCognitionRunRequest,
) (FullCognitionTransferResult, error) {
	if ctx == nil {
		return FullCognitionTransferResult{}, fmt.Errorf("full cognition transfer context is nil")
	}
	if err := validateFullCognitionTransferRequests(surfaces, requests); err != nil {
		return FullCognitionTransferResult{}, err
	}
	first := requests[0]
	authority, err := fixture.TransferAuthority(
		surfaces, first.RatGeneration, VariantFullCognition,
		first.Repetition, first.RuntimeFingerprint,
	)
	if err != nil {
		return FullCognitionTransferResult{}, err
	}
	runs := make([]FullCognitionRunResult, 0, len(requests))
	episodes := make([]TransferEpisodeResult, 0, len(requests))
	for index, request := range requests {
		run, runErr := RunFullCognition(ctx, fixture, request)
		if runErr != nil {
			return FullCognitionTransferResult{}, fmt.Errorf(
				"execute full cognition transfer surface %d: %w", index+1, runErr,
			)
		}
		bound, bindErr := BindTransferVariant(
			authority, run.Variant, run.Episode, run.Evaluation, run.CausalAcquisition,
		)
		if bindErr != nil {
			return FullCognitionTransferResult{}, fmt.Errorf(
				"bind full cognition transfer surface %d: %w", index+1, bindErr,
			)
		}
		runs = append(runs, run)
		episodes = append(episodes, bound)
	}
	report, err := EvaluateTransferRail(authority, episodes)
	if err != nil {
		return FullCognitionTransferResult{}, err
	}
	result := FullCognitionTransferResult{Authority: authority, Runs: runs, Report: report}
	return result, result.Validate()
}

func validateFullCognitionTransferRequests(
	surfaces []Surface,
	requests []FullCognitionRunRequest,
) error {
	if len(surfaces) < 2 || len(requests) != len(surfaces) {
		return fmt.Errorf("full cognition transfer requires one run request for each of at least two surfaces")
	}
	versions, err := sortedSurfaceVersions(surfaces)
	if err != nil {
		return err
	}
	requested := make(map[string]struct{}, len(requests))
	first := requests[0]
	for index, request := range requests {
		if err := request.Validate(); err != nil {
			return fmt.Errorf("full cognition transfer request %d: %w", index+1, err)
		}
		version, err := request.Surface.Version()
		if err != nil {
			return err
		}
		if _, duplicate := requested[version]; duplicate {
			return fmt.Errorf("full cognition transfer surface %q is duplicated", version)
		}
		requested[version] = struct{}{}
		if request.Repetition != first.Repetition ||
			request.RatGeneration.FixedSHA256 != first.RatGeneration.FixedSHA256 ||
			request.RatGeneration.Runtime != first.RatGeneration.Runtime ||
			request.RuntimeFingerprint != first.RuntimeFingerprint {
			return fmt.Errorf("full cognition transfer changed frozen brain, runtime, or repetition")
		}
	}
	for _, version := range versions {
		if _, exists := requested[version]; !exists {
			return fmt.Errorf("full cognition transfer omitted registered surface %q", version)
		}
	}
	return nil
}

func (result FullCognitionTransferResult) Validate() error {
	if err := result.Authority.Validate(); err != nil {
		return err
	}
	if result.Authority.Variant != VariantFullCognition || len(result.Runs) != len(result.Report.Episodes) ||
		!reflect.DeepEqual(result.Report.Authority, result.Authority) {
		return fmt.Errorf("full cognition transfer result authority is inconsistent")
	}
	for index := range result.Runs {
		if err := result.Runs[index].Validate(); err != nil {
			return fmt.Errorf("full cognition transfer run %d: %w", index+1, err)
		}
	}
	return nil
}
