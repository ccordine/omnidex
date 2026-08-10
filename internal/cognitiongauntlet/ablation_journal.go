package cognitiongauntlet

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

type ablationCall struct {
	Attempt cognitionpolicy.CallAttempt
	Result  cognitionpolicy.CallResult
}

type ablationCallJournal struct {
	mu       sync.Mutex
	attempts map[string]cognitionpolicy.CallAttempt
	results  map[string]cognitionpolicy.CallResult
	evidence map[string]cognitionpolicy.CallEvidence
	order    []string
}

func newAblationCallJournal() *ablationCallJournal {
	return &ablationCallJournal{
		attempts: make(map[string]cognitionpolicy.CallAttempt),
		results:  make(map[string]cognitionpolicy.CallResult),
		evidence: make(map[string]cognitionpolicy.CallEvidence),
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
	if err := attempt.Validate(); err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
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
	if err := result.Validate(attempt); err != nil {
		return err
	}
	if err := evidence.ValidateFor(attempt, result); err != nil {
		return fmt.Errorf("ablation call result evidence is invalid: %w", err)
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
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
	return ablationCall{Attempt: journal.attempts[id].Clone(), Result: result.Clone()}, nil
}
