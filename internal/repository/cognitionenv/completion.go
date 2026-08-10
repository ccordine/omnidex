package cognitionenv

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func (environment *Environment) Evaluate(
	ctx context.Context,
	request cognitionruntime.CompletionRequest,
) (cognition.CompletionResult, error) {
	if err := environment.validate(ctx); err != nil {
		return cognition.CompletionResult{}, err
	}
	if err := validateCompletionRequest(request); err != nil {
		return cognition.CompletionResult{}, err
	}
	if request.Binding.Episode != environment.episode ||
		request.Revision.EpisodeID != environment.episode.ID {
		return cognition.CompletionResult{}, fmt.Errorf("repository cognition completion belongs to another episode")
	}
	if err := environment.authorize(ctx, request.Binding.Attempt); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return cognition.CompletionResult{}, contextErr
		}
		return cognition.CompletionResult{}, fmt.Errorf(
			"%w: repository cognition completion actor is stale", cognition.ErrAuthorityDenied,
		)
	}
	if !reflect.DeepEqual(request.Goal, environment.investigation.goal) ||
		!reflect.DeepEqual(request.Obligation.Desired, environment.investigation.goal) ||
		request.Obligation.CompletionCheck != environment.investigation.completion.Check {
		return cognition.CompletionResult{}, fmt.Errorf(
			"%w: repository cognition completion authority changed", cognition.ErrInvalidCompletionCheck,
		)
	}
	state, err := environment.journal.EnvironmentState(
		ctx, environment.episode, environment.investigation.ref,
	)
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	if err := state.Validate(); err != nil {
		return cognition.CompletionResult{}, fmt.Errorf("repository cognition journal state: %w", err)
	}
	if request.Revision != state.Current || request.EnvironmentTerminal != state.Terminal {
		return cognition.CompletionResult{}, fmt.Errorf(
			"%w: completion request does not match durable environment state",
			cognition.ErrInvalidCompletionResult,
		)
	}
	if !request.EnvironmentTerminal {
		if request.PublicOutcome != "" {
			return cognition.CompletionResult{}, fmt.Errorf(
				"%w: nonterminal repository cognition revision changed", cognition.ErrInvalidCompletionResult,
			)
		}
		return cognition.NewCompletionResult(
			request.Obligation.ID, request.Obligation.CompletionCheck,
			request.Revision, cognition.CompletionUnsatisfied, nil,
		)
	}
	terminal := state.TerminalReceipt.Transition
	if request.PublicOutcome != terminal.PublicOutcome ||
		request.PublicOutcome != PublicOutcomeEvidenceAcquired {
		return cognition.CompletionResult{}, fmt.Errorf(
			"%w: terminal repository cognition state changed", cognition.ErrInvalidCompletionResult,
		)
	}
	evidence, err := environment.terminalEvidenceObservation(terminal)
	if err != nil {
		return cognition.CompletionResult{}, fmt.Errorf(
			"%w: terminal repository evidence receipt is invalid: %v", cognition.ErrInvalidEvidence, err,
		)
	}
	expected := evidence.EvidenceRef()
	if !containsEvidence(request.EvidenceRefs, expected) {
		return cognition.CompletionResult{}, fmt.Errorf(
			"%w: terminal repository evidence is absent", cognition.ErrInvalidEvidence,
		)
	}
	initial := state.Start.Observations[0].EvidenceRef()
	if !containsEvidence(request.EvidenceRefs, initial) {
		return cognition.CompletionResult{}, fmt.Errorf(
			"%w: accepted repository need evidence is absent", cognition.ErrInvalidEvidence,
		)
	}
	return cognition.NewCompletionResult(
		request.Obligation.ID, request.Obligation.CompletionCheck,
		request.Revision, cognition.CompletionSatisfied, []cognition.EvidenceRef{initial, expected},
	)
}

func (environment *Environment) terminalEvidenceObservation(
	transition *cognition.Transition,
) (cognition.Observation, error) {
	if transition == nil {
		return cognition.Observation{}, fmt.Errorf("terminal transition is absent")
	}
	var evidence cognition.Observation
	var state investigationState
	stateFound := false
	for _, observation := range transition.Observations {
		switch observation.Kind {
		case ObservationEvidence:
			if evidence.ID != "" {
				return cognition.Observation{}, fmt.Errorf("multiple terminal evidence packs")
			}
			evidence = observation
		case ObservationState:
			if stateFound {
				return cognition.Observation{}, fmt.Errorf("multiple terminal state observations")
			}
			if err := json.Unmarshal([]byte(observation.Content), &state); err != nil {
				return cognition.Observation{}, err
			}
			stateFound = true
		}
	}
	if evidence.ID == "" || !stateFound || state.Stage != stageComplete {
		return cognition.Observation{}, fmt.Errorf("terminal evidence or completed state is absent")
	}
	if err := state.Validate(environment.investigation); err != nil {
		return cognition.Observation{}, err
	}
	var pack repositoryretrieval.EvidencePack
	if err := json.Unmarshal([]byte(evidence.Content), &pack); err != nil {
		return cognition.Observation{}, err
	}
	if err := environment.validateTerminalPack(pack, state); err != nil {
		return cognition.Observation{}, err
	}
	return evidence, nil
}

func (environment *Environment) validateTerminalPack(
	pack repositoryretrieval.EvidencePack,
	state investigationState,
) error {
	if pack.Operation != environment.investigation.operation ||
		pack.SnapshotID != environment.investigation.snapshot.ID ||
		pack.AnalysisID != environment.investigation.analysis.ID ||
		pack.ID != state.EvidencePackID {
		return fmt.Errorf("terminal evidence differs from the registered investigation")
	}
	query := environment.investigation.query
	if pack.Operation != repositoryretrieval.OperationSemanticExcerpts {
		symbol, exists := environment.symbol(pack.SubjectSymbolID)
		if !exists || !environment.matchesAcceptedSubject(symbol) ||
			!containsString(state.InspectedRefs, symbol.ID) {
			return fmt.Errorf("terminal evidence subject is not the exact inspected query authority")
		}
		query = symbol.QualifiedName
	}
	return pack.ValidateForRequest(environment.investigation.operation, query)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validateCompletionRequest(request cognitionruntime.CompletionRequest) error {
	if err := request.Binding.Validate(); err != nil {
		return err
	}
	if !validDigest(request.SnapshotSHA256) {
		return fmt.Errorf("%w: completion snapshot hash is invalid", cognition.ErrInvalidCompletionResult)
	}
	if err := request.Goal.Validate(); err != nil {
		return err
	}
	if err := request.Revision.Validate(); err != nil {
		return err
	}
	if err := request.Obligation.Validate(); err != nil {
		return err
	}
	if request.Obligation.Status != cognition.ObligationActive {
		return fmt.Errorf("%w: completion obligation is not active", cognition.ErrInvalidCompletionResult)
	}
	seen := make(map[cognition.EvidenceRef]struct{}, len(request.EvidenceRefs))
	for index, ref := range request.EvidenceRefs {
		if err := ref.Validate(); err != nil || ref.Revision.EpisodeID != request.Binding.Episode.ID ||
			ref.Revision.Number > request.Revision.Number {
			return fmt.Errorf("%w: completion evidence %d is invalid", cognition.ErrInvalidEvidence, index)
		}
		if _, duplicate := seen[ref]; duplicate {
			return fmt.Errorf("%w: completion evidence %d is duplicated", cognition.ErrInvalidEvidence, index)
		}
		seen[ref] = struct{}{}
	}
	for index, ref := range request.Obligation.SupportingRefs {
		if _, exists := seen[ref]; !exists {
			return fmt.Errorf("%w: obligation evidence %d is absent", cognition.ErrInvalidEvidence, index)
		}
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
