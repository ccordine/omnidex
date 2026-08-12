package cognitionreference

import (
	"errors"
	"fmt"
)

var ErrInvalidSemanticFactProducer = errors.New("invalid cognition reference semantic fact producer")

type SemanticEvidenceBinding struct {
	EvidenceID EvidenceID
	FactID     FactID
}

// SemanticCandidateValue maps an opaque semantic answer to typed cognition
// state. It deliberately cannot name or invoke an operation.
type SemanticCandidateValue struct {
	CandidateID CandidateID
	Fact        Fact
}

type SemanticFactProducer struct {
	FactID           FactID
	Gap              SemanticGap
	EvidenceBindings []SemanticEvidenceBinding
	Values           []SemanticCandidateValue
}

type SemanticResolution struct {
	GapID       GapID
	CandidateID CandidateID
	Fact        Fact
}

func validateSemanticFactProducers(
	catalog Catalog,
	objective Objective,
	producers []SemanticFactProducer,
) (map[FactID]SemanticFactProducer, error) {
	registered := make(map[FactID]SemanticFactProducer, len(producers))
	for index, producer := range producers {
		if err := producer.validate(catalog, objective); err != nil {
			return nil, fmt.Errorf("%w: producer %d: %v", ErrInvalidSemanticFactProducer, index, err)
		}
		if _, duplicate := registered[producer.FactID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate producer for fact %q", ErrInvalidSemanticFactProducer, producer.FactID)
		}
		registered[producer.FactID] = producer.Clone()
	}
	return registered, nil
}

func (producer SemanticFactProducer) validate(catalog Catalog, objective Objective) error {
	definition, exists := catalog.facts[producer.FactID]
	if !exists {
		return fmt.Errorf("fact %q is not registered", producer.FactID)
	}
	if len(catalog.producers[producer.FactID]) != 0 {
		return fmt.Errorf("fact %q also has deterministic producers", producer.FactID)
	}
	if err := producer.Gap.Validate(); err != nil {
		return err
	}
	if producer.Gap.ObjectiveID != objective.ID {
		return fmt.Errorf("gap does not bind the exact objective")
	}
	if len(producer.Values) != len(producer.Gap.Candidates) {
		return fmt.Errorf("candidate values are not total")
	}
	for index, value := range producer.Values {
		if value.CandidateID != producer.Gap.Candidates[index].ID {
			return fmt.Errorf("candidate values differ from exact gap order")
		}
		if value.Fact.ID != producer.FactID || !validFactText(value.Fact.Text, definition.MaxBytes) {
			return fmt.Errorf("candidate %q fact violates its registered schema", value.CandidateID)
		}
		for prior := 0; prior < index; prior++ {
			if producer.Values[prior].Fact.Text == value.Fact.Text {
				return fmt.Errorf("candidate %q duplicates a semantic fact value", value.CandidateID)
			}
		}
	}
	if len(producer.EvidenceBindings) != len(producer.Gap.Evidence) {
		return fmt.Errorf("evidence bindings are not total")
	}
	boundFacts := make(map[FactID]struct{}, len(producer.EvidenceBindings))
	for index, binding := range producer.EvidenceBindings {
		if binding.EvidenceID != producer.Gap.Evidence[index].ID {
			return fmt.Errorf("evidence bindings differ from exact gap order")
		}
		if _, exists := catalog.facts[binding.FactID]; !exists || binding.FactID == producer.FactID {
			return fmt.Errorf("evidence %q binds an invalid source fact", binding.EvidenceID)
		}
		if _, duplicate := boundFacts[binding.FactID]; duplicate {
			return fmt.Errorf("evidence %q reuses source fact %q", binding.EvidenceID, binding.FactID)
		}
		boundFacts[binding.FactID] = struct{}{}
	}
	return nil
}

func (producer SemanticFactProducer) Clone() SemanticFactProducer {
	producer.Gap = producer.Gap.Clone()
	producer.EvidenceBindings = append([]SemanticEvidenceBinding{}, producer.EvidenceBindings...)
	producer.Values = append([]SemanticCandidateValue{}, producer.Values...)
	return producer
}

func (producer SemanticFactProducer) validateState(state State) error {
	for index, binding := range producer.EvidenceBindings {
		fact, exists := state.Fact(binding.FactID)
		if !exists || fact.Text != producer.Gap.Evidence[index].Content {
			return fmt.Errorf(
				"%w: gap evidence %q differs from code-held fact %q",
				ErrInvalidSemanticFactProducer, binding.EvidenceID, binding.FactID,
			)
		}
	}
	return nil
}

func (producer SemanticFactProducer) resolve(selected CandidateID) (SemanticResolution, error) {
	if err := producer.Gap.ValidateSelection(selected); err != nil {
		return SemanticResolution{}, err
	}
	for _, value := range producer.Values {
		if value.CandidateID == selected {
			return SemanticResolution{
				GapID: producer.Gap.ID, CandidateID: selected, Fact: value.Fact,
			}, nil
		}
	}
	return SemanticResolution{}, fmt.Errorf("%w: candidate value is missing", ErrInvalidSemanticFactProducer)
}
