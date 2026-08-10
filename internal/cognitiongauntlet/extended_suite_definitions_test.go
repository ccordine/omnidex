package cognitiongauntlet

import (
	"errors"
	"reflect"
	"testing"
)

func TestExtendedSuitesV1DefinesEveryDocumentedCapabilityExactlyOnce(t *testing.T) {
	t.Parallel()
	definitions := ExtendedSuitesV1()
	want := []Suite{
		SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder,
		SuiteResume, SuiteScale, SuiteTransfer, SuiteRogue,
	}
	got := make([]Suite, len(definitions))
	for index, definition := range definitions {
		if err := definition.Validate(); err != nil {
			t.Fatalf("definition %d: %v", index+1, err)
		}
		got[index] = definition.Suite
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extended suites=%v want=%v", got, want)
	}
	definitions[0].Prerequisites[0] = SuiteRogue
	reloaded, _ := ExtendedSuiteV1(SuiteTraverse)
	if reloaded.Prerequisites[0] == SuiteRogue {
		t.Fatal("extended suite catalog leaked mutable prerequisite state")
	}
}

func TestExtendedSuitesFailLoudlyUntilExactExecutionAuthorityExists(t *testing.T) {
	t.Parallel()
	ready := map[Suite]ExtendedSuiteExecution{
		SuiteTraverse: ExecutionScenario, SuiteBind: ExecutionScenario,
		SuiteRevise: ExecutionScenario, SuiteOrder: ExecutionScenario,
		SuiteResume: ExecutionProductionResume,
		SuiteScale:  ExecutionScaleFamily, SuiteTransfer: ExecutionTransferFamily,
	}
	for _, definition := range ExtendedSuitesV1() {
		definition := definition
		t.Run(string(definition.Suite), func(t *testing.T) {
			t.Parallel()
			loaded, err := RequireExecutableExtendedSuiteV1(definition.Suite)
			execution, executable := ready[definition.Suite]
			if executable {
				if err != nil || !loaded.Executable || loaded.Execution != execution {
					t.Fatalf("executable definition=%+v error=%v", loaded, err)
				}
				return
			}
			if !errors.Is(err, ErrExtendedSuiteUnavailable) ||
				len(definition.MissingAuthorities) == 0 {
				t.Fatalf("missing suite definition=%+v error=%v", definition, err)
			}
		})
	}
}

func TestExtendedSuiteDefinitionsRejectFalseReadinessAndIncompleteRails(t *testing.T) {
	t.Parallel()
	base, _ := ExtendedSuiteV1(SuiteTraverse)
	for name, mutate := range map[string]func(*ExtendedSuiteDefinition){
		"nil prerequisites": func(value *ExtendedSuiteDefinition) { value.Prerequisites = nil },
		"duplicate prerequisite": func(value *ExtendedSuiteDefinition) {
			value.Prerequisites[1] = value.Prerequisites[0]
		},
		"nil proofs": func(value *ExtendedSuiteDefinition) { value.RequiredProofs = nil },
		"missing ordinary runtime": func(value *ExtendedSuiteDefinition) {
			value.RequiredProofs = value.RequiredProofs[:2]
		},
		"false unavailable": func(value *ExtendedSuiteDefinition) { value.Executable = false },
		"spurious missing authority": func(value *ExtendedSuiteDefinition) {
			value.MissingAuthorities = []string{"not actually missing"}
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := base
			candidate.Prerequisites = append([]Suite(nil), base.Prerequisites...)
			candidate.RequiredProofs = append([]ExtendedSuiteProof(nil), base.RequiredProofs...)
			candidate.MissingAuthorities = append([]string(nil), base.MissingAuthorities...)
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid extended suite definition was accepted")
			}
		})
	}
}

func TestRogueDefinitionRequiresEveryPriorCapability(t *testing.T) {
	t.Parallel()
	rogue, err := ExtendedSuiteV1(SuiteRogue)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rogue.Prerequisites, roguePrerequisites()) {
		t.Fatalf("Rogue prerequisites=%v", rogue.Prerequisites)
	}
	if rogue.Executable || rogue.Execution != ExecutionRogueComposition {
		t.Fatal("Rogue definition claimed an executable composition before its runner exists")
	}
}
