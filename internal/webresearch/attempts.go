package webresearch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/specialistworkflow"
	"github.com/gryph/omnidex/internal/websearch"
)

const (
	discoveryCapability specialistworkflow.CapabilityID = "web.candidate-discovery"
	discoveryWorkflow   specialistworkflow.WorkflowID   = "websearch.discovery.v1"
	fetchCapability     specialistworkflow.CapabilityID = "web.document-fetch"
	fetchWorkflow       specialistworkflow.WorkflowID   = "websearch.explicit-fetch.v1"
)

type discoveryState struct{ query string }
type discoveryAttemptConfig struct{ query string }

type discoveryOutcome string

const (
	discoveryObservedCandidates discoveryOutcome = "candidates"
	discoveryObservedEmpty      discoveryOutcome = "empty"
)

type discoveryObservation struct {
	report  websearch.CandidateReport
	outcome discoveryOutcome
}

type fetchState struct{ candidates []websearch.Candidate }
type fetchAttemptConfig struct{ candidates []websearch.Candidate }

type fetchOutcome string

const (
	fetchObservedDocuments fetchOutcome = "documents"
	fetchObservedEmpty     fetchOutcome = "empty"
)

type fetchObservation struct {
	report  websearch.DocumentReport
	outcome fetchOutcome
}

type acquisitionFailureKind string

const (
	failureNoCandidates acquisitionFailureKind = "no_candidates"
	failureNoDocuments  acquisitionFailureKind = "no_documents"
)

type acquisitionFailure struct{ kind acquisitionFailureKind }

func (value discoveryState) ValidateBounds() error {
	return validateAcquisitionQuery(value.query)
}
func (value discoveryAttemptConfig) ValidateBounds() error {
	return validateAcquisitionQuery(value.query)
}
func (value discoveryObservation) ValidateBounds() error {
	return validateCandidateReportBounds(value.report)
}
func (value fetchState) ValidateBounds() error {
	return validateCandidateSliceBounds(value.candidates)
}
func (value fetchAttemptConfig) ValidateBounds() error {
	return validateCandidateSliceBounds(value.candidates)
}
func (value fetchObservation) ValidateBounds() error {
	return validateDocumentReportBounds(value.report)
}
func (value acquisitionFailure) ValidateBounds() error {
	switch value.kind {
	case "", failureNoCandidates, failureNoDocuments:
		return nil
	default:
		return fmt.Errorf("unknown acquisition failure kind %q", value.kind)
	}
}
func (value discoveryState) Clone() discoveryState                 { return value }
func (value discoveryAttemptConfig) Clone() discoveryAttemptConfig { return value }
func (value discoveryObservation) Clone() discoveryObservation {
	value.report = cloneCandidateReport(value.report)
	return value
}
func (value fetchState) Clone() fetchState {
	value.candidates = cloneCandidates(value.candidates)
	return value
}
func (value fetchAttemptConfig) Clone() fetchAttemptConfig {
	value.candidates = cloneCandidates(value.candidates)
	return value
}
func (value fetchObservation) Clone() fetchObservation {
	value.report = cloneDocumentReport(value.report)
	return value
}
func (value acquisitionFailure) Clone() acquisitionFailure { return value }

type acquisitionContracts struct {
	discovery specialistworkflow.Contract[
		discoveryState, discoveryAttemptConfig, discoveryObservation, acquisitionFailure,
	]
	fetch specialistworkflow.Contract[
		fetchState, fetchAttemptConfig, fetchObservation, acquisitionFailure,
	]
}

func newAcquisitionContracts(acquisition Acquisition, maxFetchCandidates int) (acquisitionContracts, error) {
	discoveryRegistration, err := specialistworkflow.NewRegistration(discoveryCapability, discoveryWorkflow, "1")
	if err != nil {
		return acquisitionContracts{}, err
	}
	fetchRegistration, err := specialistworkflow.NewRegistration(fetchCapability, fetchWorkflow, "1")
	if err != nil {
		return acquisitionContracts{}, err
	}
	registry, err := specialistworkflow.NewRegistry([]specialistworkflow.Registration{discoveryRegistration, fetchRegistration})
	if err != nil {
		return acquisitionContracts{}, err
	}
	discoveryRegistration, err = registry.Resolve(discoveryCapability)
	if err != nil {
		return acquisitionContracts{}, err
	}
	fetchRegistration, err = registry.Resolve(fetchCapability)
	if err != nil {
		return acquisitionContracts{}, err
	}
	discovery, err := newDiscoveryContract(discoveryRegistration, acquisition)
	if err != nil {
		return acquisitionContracts{}, err
	}
	fetch, err := newFetchContract(fetchRegistration, acquisition, maxFetchCandidates)
	if err != nil {
		return acquisitionContracts{}, err
	}
	return acquisitionContracts{discovery: discovery, fetch: fetch}, nil
}

func newDiscoveryContract(
	registration specialistworkflow.Registration,
	acquisition Acquisition,
) (specialistworkflow.Contract[discoveryState, discoveryAttemptConfig, discoveryObservation, acquisitionFailure], error) {
	return specialistworkflow.NewContract(
		registration,
		func(state discoveryState) (discoveryAttemptConfig, error) {
			return discoveryAttemptConfig{query: state.query}, nil
		},
		func(config discoveryAttemptConfig) error {
			if config.query == "" || config.query != strings.TrimSpace(config.query) || len(config.query) > 4_096 {
				return fmt.Errorf("query must be trimmed and contain 1..4096 bytes")
			}
			return nil
		},
		func(ctx context.Context, config discoveryAttemptConfig) (discoveryObservation, error) {
			report, err := acquisition.Discover(ctx, websearch.QueryRequest{Query: config.query})
			if boundsErr := validateCandidateReportBounds(report); boundsErr != nil {
				return discoveryObservation{}, fmt.Errorf("discovery report bounds: %w", boundsErr)
			}
			observation := discoveryObservation{report: cloneCandidateReport(report)}
			switch {
			case err == nil:
				observation.outcome = discoveryObservedCandidates
				return observation, nil
			case errors.Is(err, websearch.ErrNoCandidates):
				observation.outcome = discoveryObservedEmpty
				return observation, nil
			default:
				return discoveryObservation{}, err
			}
		},
		func(_ context.Context, config discoveryAttemptConfig, observation discoveryObservation) (bool, error) {
			var acquisitionErr error
			if observation.outcome == discoveryObservedEmpty {
				acquisitionErr = websearch.ErrNoCandidates
			}
			if err := validateCandidateReport(observation.report, config.query, acquisitionErr); err != nil {
				return false, err
			}
			switch observation.outcome {
			case discoveryObservedCandidates:
				return true, nil
			case discoveryObservedEmpty:
				return false, nil
			default:
				return false, fmt.Errorf("unknown discovery outcome %q", observation.outcome)
			}
		},
		func(_ context.Context, _ discoveryAttemptConfig, observation discoveryObservation) (acquisitionFailure, error) {
			if observation.outcome != discoveryObservedEmpty {
				return acquisitionFailure{}, fmt.Errorf("cannot reduce discovery outcome %q", observation.outcome)
			}
			return acquisitionFailure{kind: failureNoCandidates}, nil
		},
	)
}
