package labyrinth

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestGeneratedConsumerRequiresExactAcquisitionEvidence(t *testing.T) {
	t.Parallel()
	generated, err := Generate(testGeneratorConfig(SuiteRecall, 12001))
	if err != nil {
		t.Fatal(err)
	}
	oracle := generated.PrivateOracle()
	if len(oracle.EvidenceUses) == 0 {
		t.Fatal("generated case has no evidence-use authority")
	}
	use := oracle.EvidenceUses[0]
	scenario := generated.ExecutionScenario()
	episode := cognition.EpisodeRef{ID: "evidence-grounding-episode"}
	actor := cognition.AttemptRef{JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "grounding-worker"}
	environment, err := NewEnvironment(scenario, episode, func(context.Context, cognition.AttemptRef) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	transition, err := environment.Start(t.Context(), scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	startRef := transition.Observations[0].EvidenceRef()
	var acquisitionRef cognition.EvidenceRef
	for index, step := range oracle.Witness {
		schema, exists := scenario.Catalog().Schema(step.Request.Kind)
		if !exists {
			t.Fatalf("witness schema %d is absent", index)
		}
		if step.ID == use.RequiredByActionID {
			if schema.EvidencePolicy != cognition.EvidenceRequired {
				t.Fatalf("consumer evidence policy = %q", schema.EvidencePolicy)
			}
			if _, err := cognition.NewRegisteredAction(step.ID, actor, schema, step.Request, nil); !errors.Is(err, cognition.ErrInvalidEvidence) {
				t.Fatalf("consumer without evidence error = %v", err)
			}
			wrong, err := cognition.NewRegisteredAction(
				step.ID+"-wrong-evidence", actor, schema, step.Request, []cognition.EvidenceRef{startRef},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := environment.Apply(t.Context(), episode, transition.Current, wrong); !errors.Is(err, cognition.ErrInvalidEvidence) {
				t.Fatalf("consumer with unrelated evidence error = %v", err)
			}
			grounded, err := cognition.NewRegisteredAction(
				step.ID, actor, schema, step.Request, []cognition.EvidenceRef{acquisitionRef},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := environment.Apply(t.Context(), episode, transition.Current, grounded); err != nil {
				t.Fatalf("grounded consumer: %v", err)
			}
			return
		}
		action, err := cognition.NewRegisteredAction(step.ID, actor, schema, step.Request, nil)
		if err != nil {
			t.Fatalf("register witness action %d: %v", index, err)
		}
		transition, err = environment.Apply(t.Context(), episode, transition.Current, action)
		if err != nil {
			t.Fatalf("apply witness action %d: %v", index, err)
		}
		if step.ID == use.AcquisitionActionID {
			if len(transition.Observations) != 1 {
				t.Fatalf("acquisition observations = %d", len(transition.Observations))
			}
			acquisitionRef = transition.Observations[0].EvidenceRef()
		}
	}
	t.Fatal("witness did not reach the evidence consumer")
}
