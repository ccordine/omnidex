package cognitionruntime

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

type PreparedSnapshot struct {
	Snapshot               cognition.RuntimeSnapshot         `json:"snapshot"`
	ObligationGraph        cognition.ObligationGraphSnapshot `json:"obligation_graph"`
	GraphVersion           uint64                            `json:"graph_version"`
	CompletionEvidenceRefs []cognition.EvidenceRef           `json:"completion_evidence_refs"`
	EnvironmentTerminal    bool                              `json:"environment_terminal"`
	PublicOutcome          string                            `json:"public_outcome,omitempty"`
}

func (prepared PreparedSnapshot) ValidateFor(binding Binding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	if err := prepared.Snapshot.Validate(); err != nil {
		return fmt.Errorf("%w: snapshot: %v", ErrInvalidPreparedState, err)
	}
	if prepared.Snapshot.Attempt() != binding.Attempt {
		return fmt.Errorf("%w: snapshot attempt differs from the active binding", ErrInvalidPreparedState)
	}
	revision := prepared.Snapshot.CurrentRevision()
	if revision.EpisodeID != binding.Episode.ID {
		return fmt.Errorf("%w: snapshot revision belongs to another episode", ErrInvalidPreparedState)
	}
	if prepared.GraphVersion == 0 {
		return fmt.Errorf("%w: obligation graph version must be positive", ErrInvalidPreparedState)
	}
	if err := prepared.ObligationGraph.Validate(); err != nil {
		return fmt.Errorf("%w: obligation graph: %v", ErrInvalidPreparedState, err)
	}
	if err := validateSnapshotGraph(prepared); err != nil {
		return err
	}
	if err := validatePreparedCompletionEvidence(prepared); err != nil {
		return err
	}
	if prepared.PublicOutcome != "" &&
		(!utf8.ValidString(prepared.PublicOutcome) || strings.TrimSpace(prepared.PublicOutcome) != prepared.PublicOutcome ||
			strings.ContainsRune(prepared.PublicOutcome, 0) || len(prepared.PublicOutcome) > cognition.MaxPublicOutcomeBytes) {
		return fmt.Errorf("%w: public outcome must be exact bounded text", ErrInvalidPreparedState)
	}
	if prepared.EnvironmentTerminal && prepared.PublicOutcome == "" {
		return fmt.Errorf("%w: terminal environment state requires a public outcome", ErrInvalidPreparedState)
	}
	return nil
}

func validateSnapshotGraph(prepared PreparedSnapshot) error {
	current := prepared.Snapshot.CurrentObligation()
	var activeCount int
	var rootFound bool
	for _, obligation := range prepared.ObligationGraph.Obligations {
		if obligation.ID == prepared.ObligationGraph.RootID {
			rootFound = reflect.DeepEqual(obligation.Desired, prepared.Snapshot.Goal())
		}
		if obligation.Status != cognition.ObligationActive {
			continue
		}
		activeCount++
		if !reflect.DeepEqual(obligation, current) {
			return fmt.Errorf("%w: active graph obligation differs from the snapshot", ErrInvalidPreparedState)
		}
	}
	if activeCount != 1 {
		return fmt.Errorf("%w: prepared graph requires exactly one active obligation", ErrInvalidPreparedState)
	}
	if !rootFound {
		return fmt.Errorf("%w: graph root does not bind the snapshot goal", ErrInvalidPreparedState)
	}
	return nil
}

func (prepared PreparedSnapshot) clone() PreparedSnapshot {
	prepared.ObligationGraph = prepared.ObligationGraph.Clone()
	prepared.CompletionEvidenceRefs = append(
		[]cognition.EvidenceRef{}, prepared.CompletionEvidenceRefs...,
	)
	return prepared
}

func validatePreparedCompletionEvidence(prepared PreparedSnapshot) error {
	if prepared.CompletionEvidenceRefs == nil ||
		len(prepared.CompletionEvidenceRefs) > prepared.Snapshot.Budget().MaxEvidenceRefs {
		return fmt.Errorf("%w: completion evidence must be an explicit bounded packet", ErrInvalidPreparedState)
	}
	revision := prepared.Snapshot.CurrentRevision()
	available := make(map[cognition.EvidenceRef]struct{}, len(prepared.CompletionEvidenceRefs))
	for index, ref := range prepared.CompletionEvidenceRefs {
		if err := ref.Validate(); err != nil || ref.Revision.EpisodeID != revision.EpisodeID ||
			ref.Revision.Number > revision.Number {
			return fmt.Errorf("%w: completion evidence %d is invalid", ErrInvalidPreparedState, index)
		}
		if _, duplicate := available[ref]; duplicate {
			return fmt.Errorf("%w: completion evidence %d is duplicated", ErrInvalidPreparedState, index)
		}
		available[ref] = struct{}{}
	}
	for index, ref := range prepared.Snapshot.EvidenceRefs() {
		if _, exists := available[ref]; !exists {
			return fmt.Errorf(
				"%w: model evidence %d is absent from code-owned completion evidence",
				ErrInvalidPreparedState, index,
			)
		}
	}
	return nil
}
