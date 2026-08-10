package cognitiongauntlet

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func validateMicrogauntletSemantics(
	spec MicrogauntletSpec,
	generated labyrinth.GeneratedCase,
) error {
	oracle := generated.PrivateOracle()
	difficulty := generated.PublicArtifact().World.Descriptor.Difficulty
	if len(oracle.RequiredEvidence) != difficulty.EvidenceArtifacts ||
		len(oracle.CausalDAG) != spec.Generator.Difficulty.DependencyCount ||
		len(oracle.Witness) != spec.Generator.Difficulty.SolutionDepth {
		return fmt.Errorf("microgauntlet oracle does not match its frozen difficulty coordinates")
	}
	if err := validateEvidenceUseContract(spec.Generator.Suite, oracle, generated.PublicArtifact()); err != nil {
		return err
	}
	kinds := make([]cognition.ActionKind, len(oracle.Witness))
	for index, action := range oracle.Witness {
		kinds[index] = action.Request.Kind
	}
	return validateSuiteMotif(spec.Generator.Suite, kinds)
}

func validateEvidenceUseContract(
	suite labyrinth.Suite,
	oracle labyrinth.Oracle,
	public labyrinth.GeneratedScenario,
) error {
	if len(oracle.EvidenceUses) != len(oracle.RequiredEvidence) || len(oracle.EvidenceUses) == 0 {
		return fmt.Errorf("microgauntlet evidence-use coverage is incomplete")
	}
	actions := make(map[cognition.ActionID]int, len(oracle.Witness))
	for index, action := range oracle.Witness {
		actions[action.ID] = index
	}
	records := make(map[string]labyrinth.PublicRecord, len(public.World.Descriptor.Records))
	for _, record := range public.World.Descriptor.Records {
		records[string(record.ID)] = record
	}
	required := make(map[labyrinth.EvidenceIdentity]struct{}, len(oracle.RequiredEvidence))
	for _, identity := range oracle.RequiredEvidence {
		required[identity] = struct{}{}
	}
	seen := make(map[labyrinth.EvidenceIdentity]struct{}, len(oracle.EvidenceUses))
	for index, use := range oracle.EvidenceUses {
		if _, exists := required[use.Evidence]; !exists {
			return fmt.Errorf("microgauntlet evidence use %d is not required by the oracle", index+1)
		}
		if _, duplicate := seen[use.Evidence]; duplicate {
			return fmt.Errorf("microgauntlet evidence use %d duplicates an identity", index+1)
		}
		seen[use.Evidence] = struct{}{}
		acquisition, acquired := actions[use.AcquisitionActionID]
		consumer, consumed := actions[use.RequiredByActionID]
		if !acquired || !consumed || acquisition != 0 || acquisition >= consumer ||
			consumer != len(oracle.Witness)-1 {
			return fmt.Errorf("microgauntlet evidence use %d is not causally ordered", index+1)
		}
		acquisitionAction, consumerAction := oracle.Witness[acquisition], oracle.Witness[consumer]
		if err := validateEvidenceUseKinds(suite, acquisition, consumer, acquisitionAction, consumerAction); err != nil {
			return err
		}
		record, exists := records[use.Evidence.ID]
		if !exists || record.ContentSHA256 != use.Evidence.SHA256 {
			return fmt.Errorf("microgauntlet evidence use %d lacks exact public acquisition material", index+1)
		}
		switch acquisitionAction.Request.Kind {
		case "search":
			if query := actionArgumentValue(acquisitionAction.Request, "query"); query == "" || !strings.Contains(record.Content, query) {
				return fmt.Errorf("microgauntlet evidence use %d is absent from its exact search", index+1)
			}
		case "read":
			if actionArgumentValue(acquisitionAction.Request, "artifact") != use.Evidence.ID {
				return fmt.Errorf("microgauntlet evidence use %d is absent from its exact read", index+1)
			}
		}
		consumerSet := actionArgumentValue(consumerAction.Request, "evidence_set")
		if !strings.Contains(record.Content, consumerSet) {
			return fmt.Errorf("microgauntlet evidence use %d does not support its consumer", index+1)
		}
	}
	if len(seen) != len(required) {
		return fmt.Errorf("microgauntlet evidence-use coverage is incomplete")
	}
	return nil
}

func validateEvidenceUseKinds(
	suite labyrinth.Suite,
	acquisitionIndex int,
	consumerIndex int,
	acquisition labyrinth.WitnessAction,
	consumer labyrinth.WitnessAction,
) error {
	wantAcquisition := cognition.ActionKind("search")
	if suite == labyrinth.SuiteRecall {
		wantAcquisition = "read"
		if consumerIndex-acquisitionIndex < 3 {
			return fmt.Errorf("Recall evidence use lacks a delayed consumer")
		}
	}
	if acquisition.Request.Kind != wantAcquisition {
		return fmt.Errorf("microgauntlet evidence acquisition is not %q", wantAcquisition)
	}
	wantConsumer := cognition.ActionKind("take")
	switch suite {
	case labyrinth.SuiteUnlock:
		wantConsumer = "use"
	case labyrinth.SuiteMutate, labyrinth.SuiteCombined:
		wantConsumer = "write"
	}
	if consumer.Request.Kind != wantConsumer {
		return fmt.Errorf("microgauntlet evidence consumer is not %q", wantConsumer)
	}
	consumerSet := actionArgumentValue(consumer.Request, "evidence_set")
	if consumerSet == "" {
		return fmt.Errorf("microgauntlet consumer does not bind its evidence set")
	}
	if wantConsumer == "write" &&
		(actionArgumentValue(consumer.Request, "target_record") == "" ||
			actionArgumentValue(consumer.Request, "expected_sha256") == "" ||
			actionArgumentValue(consumer.Request, "mutation_value") == "") {
		return fmt.Errorf("microgauntlet terminal write lacks exact target and value authority")
	}
	return nil
}

func actionArgumentValue(request cognition.ActionRequest, name cognition.ActionArgumentName) string {
	for _, argument := range request.Arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	return ""
}

func validateSuiteMotif(suite labyrinth.Suite, kinds []cognition.ActionKind) error {
	if len(kinds) < labyrinth.MinSolutionDepth {
		return fmt.Errorf("microgauntlet witness is too short to prove a capability motif")
	}
	switch suite {
	case labyrinth.SuiteRetrieve:
		return requireOrderedKinds(suite, kinds, "search", "read")
	case labyrinth.SuiteRecall:
		firstRead := actionIndex(kinds, "read", 0)
		secondRead := actionIndex(kinds, "read", firstRead+1)
		if firstRead != 0 || secondRead >= 0 || len(kinds)-1-firstRead < 3 {
			return fmt.Errorf("Recall witness lacks one early acquisition and delayed non-reacquired use")
		}
		return nil
	case labyrinth.SuiteUnlock:
		return requireOrderedKinds(suite, kinds, "take", "use")
	case labyrinth.SuiteMutate:
		if kinds[len(kinds)-1] != "write" {
			return fmt.Errorf("Mutate witness does not end in an exact mutation")
		}
		return requireOrderedKinds(suite, kinds, "search", "read", "write")
	case labyrinth.SuiteCombined:
		return requireOrderedKinds(suite, kinds, "search", "take", "use", "read", "write")
	default:
		return fmt.Errorf("microgauntlet suite %q has no registered capability motif", suite)
	}
}

func requireOrderedKinds(
	suite labyrinth.Suite,
	kinds []cognition.ActionKind,
	required ...cognition.ActionKind,
) error {
	next := 0
	for _, kind := range kinds {
		if next < len(required) && kind == required[next] {
			next++
		}
	}
	if next != len(required) {
		return fmt.Errorf("%s witness lacks ordered operations %v", suite, required)
	}
	return nil
}

func actionIndex(kinds []cognition.ActionKind, expected cognition.ActionKind, start int) int {
	for index := start; index < len(kinds); index++ {
		if kinds[index] == expected {
			return index
		}
	}
	return -1
}
