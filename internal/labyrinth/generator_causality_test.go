package labyrinth

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestFrozenV1SuitesBindEveryEvidenceRecordToAcquisitionAndLaterUse(t *testing.T) {
	for _, config := range frozenCausalConfigs() {
		generated, err := Generate(config)
		if err != nil {
			t.Fatalf("suite=%s seed=%d: %v", config.Suite, config.Seed, err)
		}
		oracle := generated.PrivateOracle()
		expectedUses := config.Difficulty.RelevantArtifacts
		if config.Suite == SuiteRecall {
			expectedUses = 1
		}
		if len(oracle.EvidenceUses) != expectedUses {
			t.Fatalf("suite=%s evidence uses=%d", config.Suite, len(oracle.EvidenceUses))
		}
		indices := make(map[cognition.ActionID]int, len(oracle.Witness))
		for index, action := range oracle.Witness {
			indices[action.ID] = index
		}
		for index, use := range oracle.EvidenceUses {
			if use.Evidence != oracle.RequiredEvidence[index] ||
				indices[use.AcquisitionActionID] >= indices[use.RequiredByActionID] {
				t.Fatalf("suite=%s invalid evidence use %#v", config.Suite, use)
			}
		}
		if err := verifyGeneratedCausality(generated); err != nil {
			t.Fatalf("suite=%s causal proof: %v", config.Suite, err)
		}
	}
}

func TestEvidenceAcquisitionEffectAndConsumerGateCannotBeRemoved(t *testing.T) {
	generated, err := Generate(frozenCausalConfigs()[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []struct {
		name string
		edit func(*Definition, Oracle)
	}{
		{
			"acquisition effect",
			func(definition *Definition, oracle Oracle) {
				_, witness := witnessByID(oracle.Witness, oracle.EvidenceUses[0].AcquisitionActionID)
				for index := range definition.actions {
					if definition.actions[index].Schema.Kind == witness.Request.Kind {
						definition.actions[index].Effects = effectsWithoutEvidenceAcquisition(definition.actions[index].Effects)
					}
				}
			},
		},
		{
			"consumer gate",
			func(definition *Definition, oracle Oracle) {
				_, witness := witnessByID(oracle.Witness, oracle.EvidenceUses[0].RequiredByActionID)
				for index := range definition.actions {
					if definition.actions[index].Schema.Kind != witness.Request.Kind {
						continue
					}
					conditions := definition.actions[index].Preconditions[:0]
					for _, condition := range definition.actions[index].Preconditions {
						if condition.Predicate.Name != "evidence.acquired" {
							conditions = append(conditions, condition)
						}
					}
					definition.actions[index].Preconditions = conditions
				}
			},
		},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			tampered := generated
			tampered.execution = generated.execution.clone()
			mutation.edit(&tampered.execution.definition, generated.oracle)
			if err := verifyGeneratedCausality(tampered); !errors.Is(err, ErrGeneration) {
				t.Fatalf("causal validation error=%v", err)
			}
		})
	}
}

func TestOracleRejectsMissingOrNonCausalEvidenceUseAuthority(t *testing.T) {
	generated, err := Generate(frozenCausalConfigs()[0])
	if err != nil {
		t.Fatal(err)
	}
	missing := generated.PrivateOracle()
	missing.EvidenceUses = missing.EvidenceUses[:len(missing.EvidenceUses)-1]
	missing.OracleSHA256 = ""
	if err := missing.seal(); !errors.Is(err, ErrGeneration) {
		t.Fatalf("missing evidence-use error=%v", err)
	}

	nonCausal := generated.PrivateOracle()
	nonCausal.EvidenceUses[0].RequiredByActionID = nonCausal.EvidenceUses[0].AcquisitionActionID
	nonCausal.OracleSHA256 = ""
	if err := nonCausal.seal(); !errors.Is(err, ErrGeneration) {
		t.Fatalf("non-causal evidence-use error=%v", err)
	}
}

func TestMutationSuitesRejectAValidButUnlicensedExactValue(t *testing.T) {
	for _, config := range frozenCausalConfigs()[3:] {
		generated, err := Generate(config)
		if err != nil {
			t.Fatal(err)
		}
		actor := witnessActor
		episode := cognition.EpisodeRef{ID: cognition.EpisodeID("mutation-" + string(config.Suite))}
		environment, err := NewEnvironment(generated.execution, episode, func(_ context.Context, candidate cognition.AttemptRef) error {
			if candidate != actor {
				return cognition.ErrAuthorityDenied
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		transition, err := environment.Start(context.Background(), generated.public.Scenario)
		if err != nil {
			t.Fatal(err)
		}
		observations := append([]cognition.Observation(nil), transition.Observations...)
		witness := generated.oracle.Witness
		for _, step := range witness[:len(witness)-1] {
			schema, _ := generated.execution.Catalog().Schema(step.Request.Kind)
			evidence := observationEvidenceRefs(observations)
			if schema.EvidencePolicy != cognition.EvidenceRequired {
				evidence = nil
			}
			action, registerErr := cognition.NewRegisteredAction(
				step.ID, actor, schema, step.Request, evidence,
			)
			if registerErr != nil {
				t.Fatal(registerErr)
			}
			transition, err = environment.Apply(context.Background(), episode, transition.Current, action)
			if err != nil {
				t.Fatal(err)
			}
			observations = append(observations, transition.Observations...)
		}
		wrong := witness[len(witness)-1]
		for index := range wrong.Request.Arguments {
			if wrong.Request.Arguments[index].Name == mutationValueArg {
				wrong.Request.Arguments[index].Value = string(findMutationDecoy(generated.execution.definition, wrong.Request))
			}
		}
		wrong.ID = "wrong-mutation-value"
		schema, _ := generated.execution.Catalog().Schema(wrong.Request.Kind)
		action, registerErr := cognition.NewRegisteredAction(
			wrong.ID, actor, schema, wrong.Request, observationEvidenceRefs(observations),
		)
		if registerErr != nil {
			t.Fatal(registerErr)
		}
		if _, err := environment.Apply(context.Background(), episode, transition.Current, action); !errors.Is(err, ErrPrecondition) {
			t.Fatalf("suite=%s wrong mutation error=%v", config.Suite, err)
		}
	}
}

func findMutationDecoy(definition Definition, request cognition.ActionRequest) EntityID {
	current := EntityID(actionArgument(request, mutationValueArg))
	for _, entity := range definition.entities {
		if entity.Kind == mutationValueKind && entity.ID != current {
			return entity.ID
		}
	}
	panic("validated mutation definition has no decoy")
}

func frozenCausalConfigs() []GeneratorConfig {
	values := []struct {
		suite                                    Suite
		seed                                     uint64
		world                                    int
		evidence, depth, branching, dependencies int
	}{
		{SuiteRetrieve, 11_001, 40, 3, 4, 2, 2},
		{SuiteRecall, 12_001, 48, 4, 6, 2, 2},
		{SuiteUnlock, 13_001, 56, 4, 6, 3, 4},
		{SuiteMutate, 14_001, 48, 3, 5, 2, 3},
		{SuiteCombined, 15_001, 64, 5, 7, 3, 5},
	}
	configs := make([]GeneratorConfig, len(values))
	for index, value := range values {
		configs[index] = GeneratorConfig{
			Suite: value.suite, Seed: value.seed,
			Difficulty: Difficulty{
				WorldSize: value.world, RelevantArtifacts: value.evidence,
				SolutionDepth: value.depth, BranchingFactor: value.branching,
				DependencyCount: value.dependencies,
			},
			GeneratorVersion: GeneratorVersionV1, GrammarVersion: GrammarVersionV1,
			SolverStateLimit: MaxSolverStateLimit,
		}
	}
	return configs
}
