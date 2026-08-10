package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const (
	evidenceSetKind   EntityKind = "evidence-set"
	mutationValueKind EntityKind = "mutation-value"
	queryKind         EntityKind = "query"
	contentHashKind   EntityKind = "content-hash"
	evidenceSetArg               = cognition.ActionArgumentName("evidence_set")
	mutationTargetArg            = cognition.ActionArgumentName("target_record")
	mutationValueArg             = cognition.ActionArgumentName("mutation_value")
	queryArg                     = cognition.ActionArgumentName("query")
	artifactArg                  = cognition.ActionArgumentName("artifact")
	objectArg                    = cognition.ActionArgumentName("object")
	itemArg                      = cognition.ActionArgumentName("item")
	targetArg                    = cognition.ActionArgumentName("target")
	expectedSHA256Arg            = cognition.ActionArgumentName("expected_sha256")
)

type causalContract struct {
	suite            Suite
	acquisitionIndex int
	consumerIndex    int
	evidenceSet      EntityID
	requiredRecords  []EntityID
	query            EntityID
	queryDecoy       EntityID
	readArtifact     EntityID
	object           EntityID
	useTarget        EntityID
	mutationTarget   EntityID
	mutationValue    EntityID
	mutationDecoy    EntityID
	mutationExpected EntityID
	mutationCurrent  EntityID
}

func buildCausalContract(
	config GeneratorConfig,
	plan causalPlan,
	requiredRecords []EntityID,
) (causalContract, error) {
	contract := causalContract{
		suite: config.Suite, acquisitionIndex: 0,
		consumerIndex:    len(plan.macroKinds) - 1,
		evidenceSet:      EntityID(fmt.Sprintf("evidence-set-%016x", mixSeed(config.Seed))),
		requiredRecords:  append([]EntityID(nil), requiredRecords...),
		query:            EntityID(fmt.Sprintf("query-%016x", mixSeed(config.Seed^0x51ea2c7))),
		queryDecoy:       EntityID(fmt.Sprintf("query-%016x", mixSeed(config.Seed^0xd3c0a7))),
		mutationTarget:   EntityID(fmt.Sprintf("record-%03d", config.Difficulty.SolutionDepth-1)),
		mutationValue:    EntityID(fmt.Sprintf("value-%016x", mixSeed(config.Seed^0xa11ce5eed))),
		mutationDecoy:    EntityID(fmt.Sprintf("value-%016x", mixSeed(config.Seed^0xdec0de))),
		mutationExpected: EntityID(textSHA256(generatedMutationTargetContent(config.Difficulty.SolutionDepth - 1))),
	}
	contract.mutationCurrent = EntityID(textSHA256(string(contract.mutationValue)))
	if config.Suite == SuiteUnlock {
		contract.consumerIndex = firstMacroIndex(plan.macroKinds, "use")
	}
	contract.object = contract.requiredRecords[0]
	contract.readArtifact = contract.requiredRecords[0]
	if config.Suite == SuiteMutate || config.Suite == SuiteCombined {
		contract.readArtifact = contract.mutationTarget
	}
	contract.useTarget = plan.locationForKind("use")
	if err := contract.validate(plan); err != nil {
		return causalContract{}, err
	}
	return contract, nil
}

func firstMacroIndex(kinds []cognition.ActionKind, expected cognition.ActionKind) int {
	for index, kind := range kinds {
		if kind == expected {
			return index
		}
	}
	return -1
}

func (contract causalContract) validate(plan causalPlan) error {
	minimumEvidence := MinRelevantArtifacts
	if contract.suite == SuiteRecall {
		minimumEvidence = 1
	}
	if len(contract.requiredRecords) < minimumEvidence ||
		contract.acquisitionIndex < 0 || contract.consumerIndex <= contract.acquisitionIndex ||
		contract.consumerIndex >= len(plan.macroKinds) || !validSymbol(string(contract.evidenceSet)) {
		return fmt.Errorf("%w: suite has no valid evidence acquisition/consumer contract", ErrGeneration)
	}
	if !validSymbol(string(contract.query)) || !validSymbol(string(contract.queryDecoy)) ||
		contract.query == contract.queryDecoy || !validSymbol(string(contract.readArtifact)) ||
		!validSymbol(string(contract.object)) || !validSymbol(string(contract.useTarget)) {
		return fmt.Errorf("%w: suite has no valid typed action contract", ErrGeneration)
	}
	acquisition := plan.macroKinds[contract.acquisitionIndex]
	if contract.suite == SuiteRecall {
		if acquisition != "read" || contract.consumerIndex-contract.acquisitionIndex < 3 {
			return fmt.Errorf("%w: Recall requires early read and delayed evidence use", ErrGeneration)
		}
	} else if acquisition != "search" {
		return fmt.Errorf("%w: suite %s requires search acquisition", ErrGeneration, contract.suite)
	}
	consumer := plan.macroKinds[contract.consumerIndex]
	if !validSymbol(string(contract.mutationTarget)) ||
		!validSymbol(string(contract.mutationValue)) || !validSymbol(string(contract.mutationDecoy)) ||
		!validSymbol(string(contract.mutationExpected)) || !validSymbol(string(contract.mutationCurrent)) ||
		contract.mutationValue == contract.mutationDecoy || contract.mutationExpected == contract.mutationCurrent {
		return fmt.Errorf("%w: suite lacks an exact typed write contract", ErrGeneration)
	}
	if (contract.suite == SuiteMutate || contract.suite == SuiteCombined) && consumer != "write" {
		return fmt.Errorf("%w: mutation suite lacks an exact terminal write contract", ErrGeneration)
	}
	return nil
}

func mixSeed(seed uint64) uint64 {
	seed += 0x9e3779b97f4a7c15
	seed = (seed ^ (seed >> 30)) * 0xbf58476d1ce4e5b9
	seed = (seed ^ (seed >> 27)) * 0x94d049bb133111eb
	return seed ^ (seed >> 31)
}

func (contract causalContract) acquisitionKind(plan causalPlan) cognition.ActionKind {
	return plan.macroKinds[contract.acquisitionIndex]
}

func (contract causalContract) consumerKind(plan causalPlan) cognition.ActionKind {
	return plan.macroKinds[contract.consumerIndex]
}
