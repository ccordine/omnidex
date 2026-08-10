package labyrinth

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

func (oracle *Oracle) seal() error {
	oracle.canonicalize()
	digest, _, err := digestJSON(oracle.identity())
	if err != nil {
		return fmt.Errorf("%w: encode oracle identity: %v", ErrGeneration, err)
	}
	oracle.OracleSHA256 = digest
	return oracle.Validate()
}

func (oracle *Oracle) canonicalize() {
	sort.Slice(oracle.RequiredEvidence, func(left, right int) bool {
		return oracle.RequiredEvidence[left].ID < oracle.RequiredEvidence[right].ID
	})
	sort.Slice(oracle.EvidenceUses, func(left, right int) bool {
		return oracle.EvidenceUses[left].Evidence.ID < oracle.EvidenceUses[right].Evidence.ID
	})
	sort.Slice(oracle.CausalDAG, func(left, right int) bool {
		if oracle.CausalDAG[left].From != oracle.CausalDAG[right].From {
			return oracle.CausalDAG[left].From < oracle.CausalDAG[right].From
		}
		return oracle.CausalDAG[left].To < oracle.CausalDAG[right].To
	})
}

func (oracle Oracle) Validate() error {
	if oracle.Schema != OracleSchemaV2 || !validSymbol(string(oracle.ScenarioID)) ||
		!validDigest(oracle.PublicSHA256) || !validDigest(oracle.DefinitionSHA256) ||
		oracle.GeneratorVersion != GeneratorVersionV1 || oracle.GrammarVersion != GrammarVersionV1 {
		return fmt.Errorf("%w: oracle authority is invalid", ErrGeneration)
	}
	if len(oracle.Witness) < MinSolutionDepth || len(oracle.Witness) > MaxSolutionDepth {
		return fmt.Errorf("%w: oracle witness length is invalid", ErrGeneration)
	}
	witnessCost, err := validateOraclePlanActions("witness", oracle.Witness)
	if err != nil {
		return err
	}
	if witnessCost != oracle.WitnessCost || oracle.LowerBound < 1 || oracle.LowerBound > oracle.WitnessCost {
		return fmt.Errorf("%w: oracle witness costs are inconsistent", ErrGeneration)
	}
	switch oracle.Quality {
	case OracleOptimal:
		if oracle.OptimalCost == nil || len(oracle.OptimalPlan) == 0 ||
			len(oracle.OptimalPlan) > MaxSolutionDepth || *oracle.OptimalCost != oracle.LowerBound ||
			*oracle.OptimalCost > oracle.WitnessCost {
			return fmt.Errorf("%w: optimal oracle proof is inconsistent", ErrGeneration)
		}
		optimalCost, planErr := validateOraclePlanActions("optimal plan", oracle.OptimalPlan)
		if planErr != nil || optimalCost != *oracle.OptimalCost {
			return fmt.Errorf("%w: optimal oracle plan cost is inconsistent", ErrGeneration)
		}
	case OracleWitnessOnly:
		if oracle.OptimalCost != nil || oracle.OptimalPlan == nil || len(oracle.OptimalPlan) != 0 {
			return fmt.Errorf("%w: witness-only oracle claims an optimal proof", ErrGeneration)
		}
	default:
		return fmt.Errorf("%w: oracle quality is unregistered", ErrGeneration)
	}
	minimumEvidence := MinRelevantArtifacts
	if oracle.TaskArchetype == ArchetypeRecall {
		minimumEvidence = 1
	}
	if len(oracle.RequiredEvidence) < minimumEvidence || len(oracle.RequiredEvidence) > MaxRelevantArtifacts {
		return fmt.Errorf("%w: required evidence count is invalid", ErrGeneration)
	}
	previousEvidence := ""
	for _, evidence := range oracle.RequiredEvidence {
		if !validSymbol(evidence.ID) || !validDigest(evidence.SHA256) || evidence.ID <= previousEvidence {
			return fmt.Errorf("%w: required evidence identities are invalid", ErrGeneration)
		}
		previousEvidence = evidence.ID
	}
	if err := oracle.validateEvidenceUses(); err != nil {
		return err
	}
	if len(oracle.CausalDAG) < MinDependencyCount || len(oracle.CausalDAG) > MaxDependencyCount ||
		oracle.ExpandedStates < 1 || oracle.ExpandedStates > MaxSolverStateLimit ||
		!validTaskArchetype(oracle.TaskArchetype) {
		return fmt.Errorf("%w: oracle proof metadata is invalid", ErrGeneration)
	}
	if err := validateCausalDAG(oracle.CausalDAG); err != nil {
		return err
	}
	expected, _, err := digestJSON(oracle.identity())
	if err != nil || expected != oracle.OracleSHA256 {
		return fmt.Errorf("%w: oracle hash does not bind exact private content", ErrGeneration)
	}
	return nil
}

func (oracle Oracle) identity() any {
	copy := oracle
	copy.OracleSHA256 = ""
	type identity Oracle
	return identity(copy)
}

func (oracle Oracle) MarshalJSON() ([]byte, error) {
	if err := oracle.Validate(); err != nil {
		return nil, err
	}
	type wire Oracle
	raw, err := json.Marshal(wire(oracle))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxGeneratedArtifactBytes {
		return nil, fmt.Errorf("%w: private oracle exceeds %d bytes", ErrGeneration, MaxGeneratedArtifactBytes)
	}
	return raw, nil
}

func (oracle Oracle) clone() Oracle {
	oracle.Witness = cloneWitness(oracle.Witness)
	oracle.OptimalPlan = cloneWitness(oracle.OptimalPlan)
	oracle.RequiredEvidence = append([]EvidenceIdentity(nil), oracle.RequiredEvidence...)
	oracle.EvidenceUses = append([]EvidenceUse(nil), oracle.EvidenceUses...)
	oracle.CausalDAG = append([]CausalEdge(nil), oracle.CausalDAG...)
	if oracle.OptimalCost != nil {
		value := *oracle.OptimalCost
		oracle.OptimalCost = &value
	}
	return oracle
}

func validateOraclePlanActions(label string, actions []WitnessAction) (int, error) {
	if actions == nil {
		return 0, fmt.Errorf("%w: %s actions must be explicit", ErrGeneration, label)
	}
	total := 0
	seen := make(map[cognition.ActionID]struct{}, len(actions))
	for index, action := range actions {
		if !validSymbol(string(action.ID)) || action.Schema.Validate() != nil ||
			action.Request.Validate() != nil || action.Cost < 1 ||
			action.Cost > cognition.MaxTransitionCost {
			return 0, fmt.Errorf("%w: %s action %d is invalid", ErrGeneration, label, index)
		}
		if _, duplicate := seen[action.ID]; duplicate {
			return 0, fmt.Errorf("%w: %s action ID is duplicated", ErrGeneration, label)
		}
		seen[action.ID] = struct{}{}
		total += action.Cost
	}
	return total, nil
}

func (oracle Oracle) validateEvidenceUses() error {
	if len(oracle.EvidenceUses) != len(oracle.RequiredEvidence) {
		return fmt.Errorf("%w: evidence-use coverage is incomplete", ErrGeneration)
	}
	indices := make(map[cognition.ActionID]int, len(oracle.Witness))
	for index, action := range oracle.Witness {
		indices[action.ID] = index
	}
	acquisitionAuthority := oracle.EvidenceUses[0].AcquisitionActionID
	consumerAuthority := oracle.EvidenceUses[0].RequiredByActionID
	for index, use := range oracle.EvidenceUses {
		if use.Evidence != oracle.RequiredEvidence[index] {
			return fmt.Errorf("%w: evidence use %d does not bind the exact required identity", ErrGeneration, index)
		}
		acquisition, acquisitionExists := indices[use.AcquisitionActionID]
		consumer, consumerExists := indices[use.RequiredByActionID]
		if !acquisitionExists || !consumerExists || acquisition != 0 ||
			consumer != len(oracle.Witness)-1 || acquisition >= consumer ||
			use.AcquisitionActionID != acquisitionAuthority || use.RequiredByActionID != consumerAuthority {
			return fmt.Errorf("%w: evidence use %d has no strictly ordered witness actions", ErrGeneration, index)
		}
		acquisitionKind := oracle.Witness[acquisition].Request.Kind
		consumerKind := oracle.Witness[consumer].Request.Kind
		if oracle.TaskArchetype == ArchetypeRecall {
			if acquisitionKind != "read" || consumer-acquisition < 3 {
				return fmt.Errorf("%w: Recall evidence is not acquired early and consumed after a delay", ErrGeneration)
			}
		} else if acquisitionKind != "search" {
			return fmt.Errorf("%w: evidence acquisition is not a bounded search", ErrGeneration)
		}
		wantConsumer := cognition.ActionKind("take")
		switch oracle.TaskArchetype {
		case ArchetypeUnlock:
			wantConsumer = "use"
		case ArchetypeMutate, ArchetypeCombined:
			wantConsumer = "write"
		}
		if consumerKind != wantConsumer {
			return fmt.Errorf("%w: evidence does not gate the registered terminal consumer", ErrGeneration)
		}
	}
	return nil
}

func validTaskArchetype(value TaskArchetype) bool {
	switch value {
	case ArchetypeRetrieve, ArchetypeRecall, ArchetypeUnlock, ArchetypeMutate, ArchetypeCombined:
		return true
	default:
		return false
	}
}

func cloneWitness(values []WitnessAction) []WitnessAction {
	cloned := make([]WitnessAction, len(values))
	for index, value := range values {
		value.Request = value.Request.Clone()
		cloned[index] = value
	}
	return cloned
}
