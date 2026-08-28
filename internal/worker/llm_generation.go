package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type exactStationExecution struct {
	Gap                     queue.StationGapOpening
	Candidate               string
	CallReceiptSHA256       string
	CandidateResponseSHA256 string
	ProviderIdentity        llm.ProviderIdentityExpectation
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
	selection, err := providerSelectionForPortableJob(job, modelName, contextTokens)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(job, contextTokens)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"derive exact station output ceiling: %w", err,
		)
	}
	if err := validateExactStationStaticCall(prompt, contract, selection); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	opening, err := s.repo.OpenStationGapDiscovery(ctx, queue.StationGapDiscoveryOpenRecord{
		Gap: queue.StationGapOpenRecord{
			Authority: authority, Job: job, Station: stationID,
			ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens,
			OutputLimitMode: contract.OutputLimitMode,
		},
		Selection: selection,
	})
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("persist typed station gap and provider discovery: %w", err)
	}
	gap, discovery := opening.Gap, opening.Discovery
	if nilWorkerTransport(s.stationClient) {
		return s.rejectStationDiscoveryBeforeProvider(
			ctx, authority, gap, discovery, selection,
			fmt.Errorf("exact station generation provider is not configured"),
		)
	}
	if err := s.stationClient.RequireExactPreparedContract(); err != nil {
		return s.rejectStationDiscoveryBeforeProvider(
			ctx, authority, gap, discovery, selection,
			fmt.Errorf("exact station provider: %w", err),
		)
	}
	observed, discoveryErr := s.stationClient.DiscoverProviderIdentityEvidence(
		ctx, selection, discovery.Challenge,
	)
	observed, ownershipErr := ownStationDiscovery(observed)
	if ownershipErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("station discovery ownership left an unmatched boundary: %w", ownershipErr)
	}
	if discoveryErr == nil {
		expected, deriveErr := llm.DeriveExactProviderIdentityExpectation(observed.Evidence, selection)
		if deriveErr != nil {
			discoveryErr = deriveErr
		} else if validateErr := observed.ValidateFor(llm.ProviderIdentityObservationRequest{
			Expectation: expected, ChallengeSHA256: discovery.Challenge,
		}); validateErr != nil {
			discoveryErr = validateErr
		}
	}
	if discoveryErr != nil {
		failureReason, failedObservation, reasonErr := classifyStationDiscoveryFailure(
			observed, selection, discovery.Challenge, discoveryErr,
		)
		if reasonErr != nil {
			return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
				"classify exact station discovery rejection: %w", reasonErr,
			)
		}
		cause := fmt.Errorf("discover exact station provider: %w", discoveryErr)
		persistCtx, cancel := stationPersistenceContext(ctx)
		_, persistErr := s.repo.RecordStationDiscoveryFailure(persistCtx, queue.StationDiscoveryFailureRecord{
			Authority: authority, Gap: gap, Discovery: discovery,
			Observed: failedObservation, FailureReason: failureReason,
			Error: stationFailureText(cause),
		})
		cancel()
		return assemblyline.PortableResult{}, exactStationExecution{}, errors.Join(cause, persistErr)
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(observed.Evidence, selection)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	prepared, err := prepareExactStationCall(gap, contract, modelName, expected, nil)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	persistCtx, cancel := stationPersistenceContext(ctx)
	transition, err := s.repo.RecordStationDiscoveryCallOpening(persistCtx, queue.StationDiscoveryCallOpenRecord{
		Authority: authority, Gap: gap, Discovery: discovery,
		Observed: observed, Prepared: prepared,
	})
	cancel()
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("persist discovery receipt and exact station call: %w", err)
	}
	if transition.Attempt != model.StepAttemptActive {
		return s.recordAuthorityEndedExactStationCall(
			ctx, authority, gap, transition.Call, modelName, prepared, observed, transition.Attempt,
		)
	}
	return s.dispatchExactStationCall(ctx, authority, gap, transition.Call, modelName, prepared)
}

func classifyStationDiscoveryFailure(
	observed llm.ObservedProviderIdentity,
	selection llm.ProviderIdentitySelection,
	challenge string,
	discoveryErr error,
) (queue.StationDiscoveryFailureReason, llm.ObservedProviderIdentity, error) {
	if discoveryErr == nil {
		return "", llm.ObservedProviderIdentity{}, fmt.Errorf("station discovery failure requires an exact error")
	}
	if evidenceErr := observed.Evidence.ValidateFailure(selection, nil); evidenceErr == nil {
		observed.Attestation = llm.ProviderIdentityAttestation{}
		observed.Observation = llm.ProviderIdentityObservation{}
		return queue.StationDiscoveryFailureEvidenceRejected, observed, nil
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(observed.Evidence, selection)
	if err != nil {
		return "", llm.ObservedProviderIdentity{}, fmt.Errorf(
			"provider discovery error is not proven by its bounded evidence: %w", err,
		)
	}
	validationErr := observed.ValidateFor(llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	})
	if validationErr != nil {
		return queue.StationDiscoveryFailureObservationRejected, observed, nil
	}
	return queue.StationDiscoveryFailureProviderRejected, observed, nil
}
