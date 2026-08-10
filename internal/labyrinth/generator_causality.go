package labyrinth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
)

func verifyGeneratedCausality(generated GeneratedCase) error {
	if len(generated.oracle.EvidenceUses) == 0 {
		return fmt.Errorf("%w: generated suite has no evidence-use contract", ErrGeneration)
	}
	definition := generated.execution.definition
	entities, _, err := validateEntities(definition.entities)
	if err != nil {
		return err
	}
	predicates, err := validatePredicateSchemas(definition.predicateSchemas, entityKinds(entities))
	if err != nil {
		return err
	}
	acquisitionID := generated.oracle.EvidenceUses[0].AcquisitionActionID
	consumerID := generated.oracle.EvidenceUses[0].RequiredByActionID
	acquisitionIndex, acquisition := witnessByID(generated.oracle.Witness, acquisitionID)
	consumerIndex, consumer := witnessByID(generated.oracle.Witness, consumerID)
	if acquisitionIndex < 0 || consumerIndex <= acquisitionIndex {
		return fmt.Errorf("%w: evidence-use witness actions are not ordered", ErrGeneration)
	}
	set, err := evidenceSetForWitness(definition, acquisition, consumer)
	if err != nil {
		return err
	}
	if err := validateEvidenceMembers(generated, definition, acquisition, set, entities, predicates); err != nil {
		return err
	}
	if err := validateEvidenceObservationLocation(generated, acquisition, set); err != nil {
		return err
	}
	return validateSuppressedAcquisitionFails(
		generated, definition, consumerID, entities, predicates,
	)
}

func evidenceSetForWitness(
	definition Definition,
	acquisition WitnessAction,
	consumer WitnessAction,
) (EntityID, error) {
	acquisitionAction, exists := actionDefinitionForKind(definition, acquisition.Request.Kind)
	if !exists {
		return "", fmt.Errorf("%w: acquisition definition is absent", ErrGeneration)
	}
	set := EntityID("")
	for _, effect := range acquisitionAction.Effects {
		if effect.Predicate.Name == "evidence.acquired" && len(effect.Predicate.Arguments) == 1 {
			set = effect.Predicate.Arguments[0].Entity
		}
	}
	consumerSet := EntityID(actionArgument(consumer.Request, evidenceSetArg))
	if set == "" || consumerSet != set {
		return "", fmt.Errorf("%w: acquisition and consumer do not bind one evidence set", ErrGeneration)
	}
	return set, nil
}

func validateEvidenceMembers(
	generated GeneratedCase,
	definition Definition,
	acquisition WitnessAction,
	set EntityID,
	entities map[EntityID]Entity,
	predicates map[cognition.PredicateName]PredicateSchema,
) error {
	action, _ := actionDefinitionForKind(definition, acquisition.Request.Kind)
	registered, err := witnessRegisteredAction(definition, acquisition)
	if err != nil {
		return err
	}
	for _, evidence := range generated.oracle.RequiredEvidence {
		member, err := generatedPredicate("evidence.member", set, EntityID(evidence.ID))
		if err != nil {
			return err
		}
		if !newFactSet(definition.initialFacts).contains(member) || !actionRequires(action, registered, member) {
			return fmt.Errorf("%w: evidence %s is not an acquisition precondition", ErrGeneration, evidence.ID)
		}
		without := newFactSet(definition.initialFacts)
		delete(without, predicateKey(member))
		if _, _, applyErr := applyActionDefinition(action, registered, entities, predicates, without); !errors.Is(applyErr, ErrPrecondition) {
			return fmt.Errorf("%w: evidence %s can be omitted from acquisition", ErrGeneration, evidence.ID)
		}
	}
	return nil
}

func validateEvidenceObservationLocation(
	generated GeneratedCase,
	acquisition WitnessAction,
	set EntityID,
) error {
	records := make(map[string]PublicRecord, len(generated.execution.descriptor.Records))
	for _, record := range generated.execution.descriptor.Records {
		records[string(record.ID)] = record
	}
	for _, evidence := range generated.oracle.RequiredEvidence {
		record, exists := records[evidence.ID]
		if !exists || record.ContentSHA256 != evidence.SHA256 || !strings.Contains(record.Content, string(set)) {
			return fmt.Errorf("%w: evidence %s is not observable at acquisition", ErrGeneration, evidence.ID)
		}
		switch acquisition.Request.Kind {
		case "search":
			query := actionArgument(acquisition.Request, queryArg)
			if query == "" || !strings.Contains(record.Content, query) {
				return fmt.Errorf("%w: evidence %s does not match its exact search query", ErrGeneration, evidence.ID)
			}
		case "read":
			if actionArgument(acquisition.Request, artifactArg) != evidence.ID {
				return fmt.Errorf("%w: Recall acquisition does not read its exact evidence artifact", ErrGeneration)
			}
		default:
			return fmt.Errorf("%w: evidence acquisition kind is unregistered", ErrGeneration)
		}
	}
	return nil
}

func validateSuppressedAcquisitionFails(
	generated GeneratedCase,
	definition Definition,
	consumerID cognition.ActionID,
	entities map[EntityID]Entity,
	predicates map[cognition.PredicateName]PredicateSchema,
) error {
	facts := newFactSet(definition.initialFacts)
	for _, witness := range generated.oracle.Witness {
		action, exists := actionDefinitionForKind(definition, witness.Request.Kind)
		if !exists {
			return fmt.Errorf("%w: witness action definition is absent", ErrGeneration)
		}
		action.Effects = effectsWithoutEvidenceAcquisition(action.Effects)
		registered, err := witnessRegisteredAction(definition, witness)
		if err != nil {
			return err
		}
		candidate, _, applyErr := applyActionDefinition(action, registered, entities, predicates, facts)
		if witness.ID == consumerID {
			if !errors.Is(applyErr, ErrPrecondition) || !onlyMissingEvidenceSet(action, registered, facts) ||
				goalSatisfied(definition.goal, facts) {
				return fmt.Errorf("%w: terminal consumer is not causally gated by acquired evidence", ErrGeneration)
			}
			return nil
		}
		if applyErr != nil {
			return fmt.Errorf(
				"%w: suppressed witness %s (%s) failed before its evidence consumer: %v",
				ErrGeneration, witness.ID, witness.Request.Kind, applyErr,
			)
		}
		facts = candidate
	}
	return fmt.Errorf("%w: evidence consumer was never reached", ErrGeneration)
}

func onlyMissingEvidenceSet(action ActionDefinition, registered cognition.RegisteredAction, facts factSet) bool {
	bindings := make(map[cognition.ActionArgumentName]EntityID, len(registered.Request.Arguments))
	for _, argument := range registered.Request.Arguments {
		bindings[argument.Name] = EntityID(argument.Value)
	}
	missing := 0
	for _, condition := range action.Preconditions {
		predicate, err := groundPattern(condition.Predicate, bindings)
		if err != nil {
			return false
		}
		present := facts.contains(predicate)
		failed := condition.Mode == ConditionPresent && !present || condition.Mode == ConditionAbsent && present
		if failed {
			missing++
			if predicate.Name != "evidence.acquired" {
				return false
			}
		}
	}
	return missing == 1
}

func effectsWithoutEvidenceAcquisition(values []Effect) []Effect {
	result := make([]Effect, 0, len(values))
	for _, effect := range values {
		if effect.Predicate.Name != "evidence.acquired" {
			result = append(result, effect)
		}
	}
	return result
}

func actionRequires(
	action ActionDefinition,
	registered cognition.RegisteredAction,
	predicate cognition.Predicate,
) bool {
	bindings := make(map[cognition.ActionArgumentName]EntityID, len(registered.Request.Arguments))
	for _, argument := range registered.Request.Arguments {
		bindings[argument.Name] = EntityID(argument.Value)
	}
	for _, condition := range action.Preconditions {
		grounded, err := groundPattern(condition.Predicate, bindings)
		if err == nil && condition.Mode == ConditionPresent && predicateKey(grounded) == predicateKey(predicate) {
			return true
		}
	}
	return false
}

func witnessRegisteredAction(definition Definition, witness WitnessAction) (cognition.RegisteredAction, error) {
	action, exists := actionDefinitionForKind(definition, witness.Request.Kind)
	if !exists {
		return cognition.RegisteredAction{}, fmt.Errorf("%w: witness definition is absent", ErrGeneration)
	}
	registered, err := cognition.NewRegisteredAction(
		witness.ID, witnessActor, action.Schema, witness.Request, generationEvidenceRefs(action.Schema),
	)
	if err != nil {
		return cognition.RegisteredAction{}, fmt.Errorf("%w: bind witness action: %v", ErrGeneration, err)
	}
	return registered, nil
}

func witnessByID(values []WitnessAction, id cognition.ActionID) (int, WitnessAction) {
	for index, action := range values {
		if action.ID == id {
			return index, action
		}
	}
	return -1, WitnessAction{}
}

func actionArgument(request cognition.ActionRequest, name cognition.ActionArgumentName) string {
	for _, argument := range request.Arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	return ""
}
