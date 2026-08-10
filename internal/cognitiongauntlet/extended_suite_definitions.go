package cognitiongauntlet

import (
	"errors"
	"fmt"
)

const ExtendedSuiteDefinitionSchemaV1 = "omnidex.cognition-extended-suite.v1"
const ExtendedSuiteFixtureVersionV1 = "extended-suites.v1"

var ErrExtendedSuiteUnavailable = errors.New("extended cognition suite is not executable")

type ExtendedSuiteExecution string

const (
	ExecutionScenario         ExtendedSuiteExecution = "scenario"
	ExecutionProductionResume ExtendedSuiteExecution = "production_resume"
	ExecutionScaleFamily      ExtendedSuiteExecution = "scale_family"
	ExecutionTransferFamily   ExtendedSuiteExecution = "transfer_family"
	ExecutionRogueComposition ExtendedSuiteExecution = "rogue_composition"
)

type ExtendedSuiteProof string

const (
	ProofPublicPrivateSeparation ExtendedSuiteProof = "public_private_separation"
	ProofValidInvalidRails       ExtendedSuiteProof = "valid_invalid_rails"
	ProofOrdinaryRuntime         ExtendedSuiteProof = "ordinary_runtime"
)

type ExtendedSuiteDefinition struct {
	Schema             string                 `json:"schema"`
	Suite              Suite                  `json:"suite"`
	FixtureVersion     string                 `json:"fixture_version"`
	Capability         string                 `json:"capability"`
	Execution          ExtendedSuiteExecution `json:"execution"`
	Prerequisites      []Suite                `json:"prerequisites"`
	RequiredProofs     []ExtendedSuiteProof   `json:"required_proofs"`
	Executable         bool                   `json:"executable"`
	MissingAuthorities []string               `json:"missing_authorities"`
}

func ExtendedSuitesV1() []ExtendedSuiteDefinition {
	initial := initialSuitePrerequisites()
	definitions := []ExtendedSuiteDefinition{
		extendedDefinition(SuiteTraverse, "partial map construction and backtracking", ExecutionScenario, initial, true),
		extendedDefinition(SuiteBind, "distant evidence binding", ExecutionScenario, initial, true),
		extendedDefinition(SuiteRevise, "contradiction-driven belief revision and replanning", ExecutionScenario, initial, true),
		extendedDefinition(SuiteOrder, "ordered and irreversible action discipline", ExecutionScenario, initial, true),
		extendedDefinition(SuiteResume, "process interruption and attempt takeover", ExecutionProductionResume, initial, true),
		extendedDefinition(SuiteScale, "fixed relevant surface under world growth", ExecutionScaleFamily, initial, true),
		extendedDefinition(SuiteTransfer, "unchanged cognition across environment surfaces", ExecutionTransferFamily, initial, true),
		extendedDefinition(SuiteRogue, "combined long-horizon dynamic composition", ExecutionRogueComposition, roguePrerequisites(), false, "Rogue executable composition authority"),
	}
	return cloneExtendedDefinitions(definitions)
}

func ExtendedSuiteV1(suite Suite) (ExtendedSuiteDefinition, error) {
	for _, definition := range ExtendedSuitesV1() {
		if definition.Suite == suite {
			return definition, nil
		}
	}
	return ExtendedSuiteDefinition{}, fmt.Errorf("extended cognition suite %q is not registered", suite)
}

func RequireExecutableExtendedSuiteV1(suite Suite) (ExtendedSuiteDefinition, error) {
	definition, err := ExtendedSuiteV1(suite)
	if err != nil {
		return ExtendedSuiteDefinition{}, err
	}
	if err := definition.Validate(); err != nil {
		return ExtendedSuiteDefinition{}, err
	}
	if !definition.Executable {
		return ExtendedSuiteDefinition{}, fmt.Errorf(
			"%w: %s", ErrExtendedSuiteUnavailable, definition.MissingAuthorities[0],
		)
	}
	return definition, nil
}

func (definition ExtendedSuiteDefinition) Validate() error {
	if definition.Schema != ExtendedSuiteDefinitionSchemaV1 ||
		definition.FixtureVersion != ExtendedSuiteFixtureVersionV1 ||
		!extendedSuite(definition.Suite) {
		return fmt.Errorf("extended suite schema, fixture, or suite is invalid")
	}
	if err := requireExact(definition.Capability, "extended suite capability", 256); err != nil {
		return err
	}
	if !validExtendedExecution(definition.Execution) || definition.Prerequisites == nil ||
		definition.RequiredProofs == nil || definition.MissingAuthorities == nil {
		return fmt.Errorf("extended suite execution and arrays must be explicit")
	}
	if !uniqueSuites(definition.Prerequisites) || !requiredExtendedProofs(definition.RequiredProofs) {
		return fmt.Errorf("extended suite prerequisites or proof rails are invalid")
	}
	for _, missing := range definition.MissingAuthorities {
		if err := requireExact(missing, "missing extended suite authority", 256); err != nil {
			return err
		}
	}
	if definition.Executable == (len(definition.MissingAuthorities) > 0) {
		return fmt.Errorf("extended suite executable status contradicts missing authority")
	}
	return nil
}

func extendedDefinition(
	suite Suite,
	capability string,
	execution ExtendedSuiteExecution,
	prerequisites []Suite,
	executable bool,
	missing ...string,
) ExtendedSuiteDefinition {
	return ExtendedSuiteDefinition{
		Schema: ExtendedSuiteDefinitionSchemaV1, Suite: suite,
		FixtureVersion: ExtendedSuiteFixtureVersionV1, Capability: capability,
		Execution: execution, Prerequisites: append([]Suite(nil), prerequisites...),
		RequiredProofs: []ExtendedSuiteProof{
			ProofPublicPrivateSeparation, ProofValidInvalidRails, ProofOrdinaryRuntime,
		},
		Executable: executable, MissingAuthorities: append([]string{}, missing...),
	}
}

func initialSuitePrerequisites() []Suite {
	return []Suite{SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined}
}

func roguePrerequisites() []Suite {
	return []Suite{
		SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder,
		SuiteResume, SuiteScale, SuiteTransfer,
	}
}

func cloneExtendedDefinitions(values []ExtendedSuiteDefinition) []ExtendedSuiteDefinition {
	result := make([]ExtendedSuiteDefinition, len(values))
	for index, value := range values {
		value.Prerequisites = append([]Suite(nil), value.Prerequisites...)
		value.RequiredProofs = append([]ExtendedSuiteProof(nil), value.RequiredProofs...)
		value.MissingAuthorities = append([]string{}, value.MissingAuthorities...)
		result[index] = value
	}
	return result
}

func extendedSuite(suite Suite) bool {
	return containsSuite([]Suite{
		SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder,
		SuiteResume, SuiteScale, SuiteTransfer, SuiteRogue,
	}, suite)
}

func validExtendedExecution(execution ExtendedSuiteExecution) bool {
	switch execution {
	case ExecutionScenario, ExecutionProductionResume, ExecutionScaleFamily,
		ExecutionTransferFamily, ExecutionRogueComposition:
		return true
	default:
		return false
	}
}

func uniqueSuites(values []Suite) bool {
	seen := make(map[Suite]struct{}, len(values))
	for _, value := range values {
		if !validSuite(value) {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return len(values) > 0
}

func requiredExtendedProofs(values []ExtendedSuiteProof) bool {
	if len(values) != 3 {
		return false
	}
	return values[0] == ProofPublicPrivateSeparation &&
		values[1] == ProofValidInvalidRails && values[2] == ProofOrdinaryRuntime
}
