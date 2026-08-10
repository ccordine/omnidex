package labyrinth

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

func (oracle *ExtendedOracle) seal() error {
	oracle.canonicalize()
	digest, _, err := digestJSON(oracle.identity())
	if err != nil {
		return fmt.Errorf("%w: encode extended oracle identity: %v", ErrGeneration, err)
	}
	oracle.OracleSHA256 = digest
	return oracle.Validate()
}

func (oracle *ExtendedOracle) canonicalize() {
	sort.Slice(oracle.InvalidRails, func(left, right int) bool {
		return oracle.InvalidRails[left].ID < oracle.InvalidRails[right].ID
	})
	sort.Slice(oracle.EvidenceUses, func(left, right int) bool {
		if oracle.EvidenceUses[left].RequiredByActionID != oracle.EvidenceUses[right].RequiredByActionID {
			return oracle.EvidenceUses[left].RequiredByActionID < oracle.EvidenceUses[right].RequiredByActionID
		}
		return oracle.EvidenceUses[left].Evidence.ID < oracle.EvidenceUses[right].Evidence.ID
	})
	sort.Slice(oracle.OmissionRails, func(left, right int) bool {
		return oracle.OmissionRails[left].ID < oracle.OmissionRails[right].ID
	})
}

func (oracle ExtendedOracle) Validate() error {
	if oracle.Schema != ExtendedOracleSchemaV1 || !validSymbol(string(oracle.ScenarioID)) ||
		!validDigest(oracle.PublicSHA256) || !validDigest(oracle.DefinitionSHA256) ||
		oracle.GeneratorVersion != ExtendedGeneratorVersionV1 ||
		oracle.GrammarVersion != ExtendedGrammarVersionV1 || !extendedLabyrinthSuite(oracle.Suite) ||
		oracle.Seed == 0 || oracle.TaskArchetype != extendedArchetype(oracle.Suite) ||
		oracle.Witness == nil || oracle.EvidenceUses == nil || oracle.InvalidRails == nil ||
		oracle.OmissionRails == nil ||
		len(oracle.Witness) < 4 || len(oracle.Witness) > MaxEpisodeTransitions ||
		len(oracle.InvalidRails) == 0 || len(oracle.OmissionRails) == 0 {
		return fmt.Errorf("%w: extended oracle authority is invalid", ErrGeneration)
	}
	actions := make(map[cognition.ActionID]int, len(oracle.Witness))
	for index, action := range oracle.Witness {
		if !validSymbol(string(action.ID)) || action.Schema.Validate() != nil ||
			action.Request.Validate() != nil || action.Cost < 1 || action.Cost > cognition.MaxTransitionCost {
			return fmt.Errorf("%w: extended witness action %d is invalid", ErrGeneration, index)
		}
		if _, duplicate := actions[action.ID]; duplicate {
			return fmt.Errorf("%w: extended witness action identity repeats", ErrGeneration)
		}
		actions[action.ID] = index
	}
	if err := validateExtendedEvidenceUses(oracle.EvidenceUses, actions); err != nil {
		return err
	}
	previousRail := ""
	for index, rail := range oracle.InvalidRails {
		if !validSymbol(rail.ID) || index > 0 && rail.ID <= previousRail || rail.PrefixActions < 0 ||
			rail.PrefixActions >= len(oracle.Witness) || rail.Request.Validate() != nil ||
			!validExtendedRailOutcome(rail) {
			return fmt.Errorf("%w: extended invalid rail %d is invalid", ErrGeneration, index)
		}
		previousRail = rail.ID
	}
	previousRail = ""
	for index, rail := range oracle.OmissionRails {
		if !validSymbol(rail.ID) || index > 0 && rail.ID <= previousRail ||
			rail.OmittedAction < 0 || rail.OmittedAction >= len(oracle.Witness) ||
			rail.FailureAction <= rail.OmittedAction || rail.FailureAction >= len(oracle.Witness) ||
			rail.OmittedKind != oracle.Witness[rail.OmittedAction].Request.Kind ||
			(rail.FailureCode != cognition.ActionFailureInvalidAction &&
				rail.FailureCode != cognition.ActionFailurePreconditionFailed) {
			return fmt.Errorf("%w: extended omission rail %d is invalid", ErrGeneration, index)
		}
		previousRail = rail.ID
	}
	expected, _, err := digestJSON(oracle.identity())
	if err != nil || expected != oracle.OracleSHA256 {
		return fmt.Errorf("%w: extended oracle hash does not bind exact private content", ErrGeneration)
	}
	return nil
}

func validExtendedRailOutcome(rail ExtendedInvalidRail) bool {
	switch rail.Outcome {
	case ExtendedRailRejected:
		return rail.FailureCode == cognition.ActionFailureInvalidAction ||
			rail.FailureCode == cognition.ActionFailurePreconditionFailed
	case ExtendedRailIrreversibleDeadEnd:
		return rail.FailureCode == ""
	default:
		return false
	}
}

func validateExtendedEvidenceUses(values []EvidenceUse, actions map[cognition.ActionID]int) error {
	seen := make(map[string]struct{}, len(values))
	for index, use := range values {
		acquisition, acquired := actions[use.AcquisitionActionID]
		consumer, consumed := actions[use.RequiredByActionID]
		key := use.Evidence.ID + "\x00" + string(use.RequiredByActionID)
		if !validSymbol(use.Evidence.ID) || !validDigest(use.Evidence.SHA256) || !acquired || !consumed ||
			acquisition >= consumer {
			return fmt.Errorf("%w: extended evidence use %d is invalid", ErrGeneration, index)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%w: extended evidence use %d repeats", ErrGeneration, index)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (oracle ExtendedOracle) identity() any {
	copy := oracle
	copy.OracleSHA256 = ""
	type identity ExtendedOracle
	return identity(copy)
}

func (oracle ExtendedOracle) MarshalJSON() ([]byte, error) {
	if err := oracle.Validate(); err != nil {
		return nil, err
	}
	type wire ExtendedOracle
	return json.Marshal(wire(oracle))
}

func (oracle ExtendedOracle) clone() ExtendedOracle {
	oracle.Witness = cloneWitness(oracle.Witness)
	oracle.EvidenceUses = append([]EvidenceUse{}, oracle.EvidenceUses...)
	oracle.OmissionRails = append([]ExtendedOmissionRail{}, oracle.OmissionRails...)
	rails := oracle.InvalidRails
	oracle.InvalidRails = make([]ExtendedInvalidRail, len(rails))
	for index, rail := range rails {
		rail.Request = rail.Request.Clone()
		oracle.InvalidRails[index] = rail
	}
	return oracle
}

func extendedArchetype(suite Suite) string {
	switch suite {
	case SuiteTraverse:
		return "partial-map-backtracking"
	case SuiteBind:
		return "distant-evidence-binding"
	case SuiteRevise:
		return "contradiction-revision-replanning"
	case SuiteOrder:
		return "ordered-irreversible-actions"
	case SuiteRogue:
		return "combined-long-horizon"
	default:
		return ""
	}
}
