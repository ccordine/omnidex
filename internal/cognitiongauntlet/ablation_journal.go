package cognitiongauntlet

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
)

type ablationCall struct {
	Projection contextbuilder.Projection
	Snapshot   semanticRuntimeSnapshot
	Attempt    cognitionpolicy.CallAttempt
	Result     cognitionpolicy.CallResult
	Evidence   cognitionpolicy.CallEvidence
}

type ablationCallInput struct {
	Projection contextbuilder.Projection
	Snapshot   semanticRuntimeSnapshot
}

type ablationCallJournal struct {
	mu       sync.Mutex
	attempts map[string]cognitionpolicy.CallAttempt
	results  map[string]cognitionpolicy.CallResult
	evidence map[string]cognitionpolicy.CallEvidence
	inputs   map[string]ablationCallInput
	order    []string
	closed   bool
}

func newAblationCallJournal() *ablationCallJournal {
	return &ablationCallJournal{
		attempts: make(map[string]cognitionpolicy.CallAttempt),
		results:  make(map[string]cognitionpolicy.CallResult),
		evidence: make(map[string]cognitionpolicy.CallEvidence),
		inputs:   make(map[string]ablationCallInput),
		order:    make([]string, 0, 32),
	}
}

func (journal *ablationCallJournal) Start(
	_ context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	if journal == nil {
		return cognitionpolicy.CallReservation{}, fmt.Errorf("ablation call journal is nil")
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return cognitionpolicy.CallReservation{}, fmt.Errorf("ablation call journal is sealed")
	}
	if err := attempt.Validate(); err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	if existing, found := journal.attempts[attempt.ID]; found {
		if !reflect.DeepEqual(existing, attempt) {
			return cognitionpolicy.CallReservation{}, fmt.Errorf("ablation call identity was reused with different authority")
		}
		reservation := cognitionpolicy.CallReservation{Attempt: existing, Created: false}
		if result, complete := journal.results[attempt.ID]; complete {
			copy := result.Clone()
			reservation.ExistingResult = &copy
			evidence, exists := journal.evidence[attempt.ID]
			if !exists {
				return cognitionpolicy.CallReservation{}, fmt.Errorf(
					"ablation call result lacks exact call evidence",
				)
			}
			if result.Status == cognitionpolicy.CallResultAccepted {
				cloned := evidence.Response.Clone()
				reservation.ExistingResponseEvidence = &cloned
			}
		}
		return reservation, reservation.ValidateFor(attempt)
	}
	journal.attempts[attempt.ID] = attempt.Clone()
	journal.order = append(journal.order, attempt.ID)
	reservation := cognitionpolicy.CallReservation{Attempt: attempt.Clone(), Created: true}
	return reservation, reservation.ValidateFor(attempt)
}

func (journal *ablationCallJournal) Finish(
	_ context.Context,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.CallEvidence,
) error {
	if journal == nil {
		return fmt.Errorf("ablation call journal is nil")
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return fmt.Errorf("ablation call journal is sealed")
	}
	if err := result.Validate(attempt); err != nil {
		return err
	}
	if err := evidence.ValidateFor(attempt, result); err != nil {
		return fmt.Errorf("ablation call result evidence is invalid: %w", err)
	}
	existing, found := journal.attempts[attempt.ID]
	if !found || !reflect.DeepEqual(existing, attempt) {
		return fmt.Errorf("ablation call result lacks its exact attempt")
	}
	if prior, duplicate := journal.results[attempt.ID]; duplicate {
		priorEvidence, hasEvidence := journal.evidence[attempt.ID]
		if reflect.DeepEqual(prior, result) &&
			hasEvidence && reflect.DeepEqual(priorEvidence, evidence) {
			return nil
		}
		return fmt.Errorf("ablation call result was replaced")
	}
	journal.results[attempt.ID] = result.Clone()
	journal.evidence[attempt.ID] = evidence.Clone()
	return nil
}

func (journal *ablationCallJournal) completed(index int) (ablationCall, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if index < 0 || index >= len(journal.order) {
		return ablationCall{}, fmt.Errorf("ablation call %d was not reserved", index+1)
	}
	id := journal.order[index]
	result, complete := journal.results[id]
	if !complete {
		return ablationCall{}, fmt.Errorf("ablation call %d has no terminal result", index+1)
	}
	evidence, exists := journal.evidence[id]
	if !exists {
		return ablationCall{}, fmt.Errorf("ablation call %d lacks exact call evidence", index+1)
	}
	return ablationCall{
		Attempt: journal.attempts[id].Clone(), Result: result.Clone(), Evidence: evidence.Clone(),
	}, nil
}

func (journal *ablationCallJournal) freeze() ([]ablationCall, error) {
	if journal == nil {
		return nil, fmt.Errorf("ablation call journal is nil")
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return nil, fmt.Errorf("ablation call journal is sealed")
	}
	if len(journal.attempts) != len(journal.order) || len(journal.results) != len(journal.order) ||
		len(journal.inputs) != len(journal.order) ||
		len(journal.evidence) != len(journal.order) {
		for index, id := range journal.order {
			if _, exists := journal.attempts[id]; !exists {
				return nil, fmt.Errorf("ablation call %d lacks its exact attempt", index+1)
			}
			if _, exists := journal.results[id]; !exists {
				return nil, fmt.Errorf("ablation call %d has no terminal result", index+1)
			}
			if _, exists := journal.evidence[id]; !exists {
				return nil, fmt.Errorf("ablation call %d lacks exact call evidence", index+1)
			}
			if _, exists := journal.inputs[id]; !exists {
				return nil, fmt.Errorf("ablation call %d lacks exact runtime input", index+1)
			}
		}
		return nil, fmt.Errorf("ablation call journal contains unjournaled authority")
	}
	calls := make([]ablationCall, len(journal.order))
	for index, id := range journal.order {
		attempt, attemptExists := journal.attempts[id]
		result, resultExists := journal.results[id]
		evidence, evidenceExists := journal.evidence[id]
		input, inputExists := journal.inputs[id]
		if !attemptExists || !resultExists || !evidenceExists || !inputExists {
			return nil, fmt.Errorf("ablation call %d is incomplete", index+1)
		}
		snapshot, err := input.Snapshot.runtimeSnapshot()
		if err != nil {
			return nil, fmt.Errorf("ablation call %d runtime snapshot is invalid: %w", index+1, err)
		}
		if err := cognitionpolicy.VerifyCallAttempt(snapshot, input.Projection, attempt); err != nil {
			return nil, fmt.Errorf("ablation call %d runtime input is invalid: %w", index+1, err)
		}
		if err := result.Validate(attempt); err != nil {
			return nil, fmt.Errorf("ablation call %d result: %w", index+1, err)
		}
		if err := evidence.ValidateFor(attempt, result); err != nil {
			return nil, fmt.Errorf("ablation call %d evidence: %w", index+1, err)
		}
		calls[index] = ablationCall{
			Projection: cloneAblationProjection(input.Projection), Snapshot: input.Snapshot.clone(),
			Attempt: attempt.Clone(), Result: result.Clone(), Evidence: evidence.Clone(),
		}
	}
	journal.closed = true
	return calls, nil
}
