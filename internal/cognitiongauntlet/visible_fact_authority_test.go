package cognitiongauntlet

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestVisibleFactAuthorityAcceptsPublicRecordsAcrossUnrelatedWorldsAndSurfaces(t *testing.T) {
	tests := []struct {
		name    string
		spec    int
		surface Surface
	}{
		{name: "retrieve-symbolic", spec: 0, surface: SurfaceSymbolic},
		{name: "recall-record", spec: 1, surface: SurfaceRecord},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[test.spec])
			if err != nil {
				t.Fatal(err)
			}
			transition, action := firstVisibleFactTransition(t, fixture, test.surface, index+1)
			authority, err := newVisibleObservationFactAuthority()
			if err != nil {
				t.Fatal(err)
			}
			ledger := newVisibleFactLedger(t, int64(index+1))
			for _, observation := range transition.Observations {
				schema, exists := fixture.SealedEnvironmentScenario().Catalog().Schema(action.Request.Kind)
				if !exists {
					t.Fatal("accepted action schema disappeared")
				}
				mutation, err := cognitionstate.MapEnvironmentObservation(cognitionstate.EnvironmentObservationInput{
					Ledger: ledger.MaterializedState(), Observation: observation,
					Action: &cognitionstate.ActionBinding{Action: action, Schema: schema},
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ledger.Apply(mutation.Command()); err != nil {
					t.Fatal(err)
				}
			}
			facts, err := authority.MapTransitionFacts(ledger.MaterializedState(), "", transition)
			if err != nil || len(facts) != 1 {
				t.Fatalf("accepted facts=%d error=%v", len(facts), err)
			}
			command := facts[0].Command()
			if command.Kind != taskstate.EntryFact || command.Actor != taskstate.AuthorityCode ||
				len(command.Refs) != 1 || command.Refs[0].Hash != transition.Observations[0].ContentSHA256 {
				t.Fatalf("accepted fact authority=%+v", command)
			}
			fact := visibleRecordFact{}
			if err := decodeStrictJSON([]byte(command.Content), &fact, "visible fact test"); err != nil ||
				fact.Schema != visibleFactSchemaV1 || fact.Source != transition.Observations[0].EvidenceRef() ||
				len(fact.Records) == 0 || fact.Records[0].Content == "" ||
				visibleFactDigest(fact.Records[0].Content) != fact.Records[0].ContentSHA256 {
				t.Fatalf("visible fact=%+v error=%v", fact, err)
			}
			lower := strings.ToLower(command.Content)
			for _, forbidden := range []string{"gauntlet", "oracle", "benchmark", "witness", "hidden"} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("model-visible accepted fact contains %q: %s", forbidden, command.Content)
				}
			}
		})
	}
}

func TestVisibleFactAuthorityUsesNoPrivateEvaluationInputs(t *testing.T) {
	raw, err := os.ReadFile("visible_fact_authority.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"PrivateOracle", "RequiredEvidence", "EvidenceUse", "TaskArchetype", "GeneratorConfig",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("visible fact authority contains private input %q", forbidden)
		}
	}
}

func TestVisibleFactAuthorityRejectsAmbiguousOrHashInvalidObservation(t *testing.T) {
	authority, err := newVisibleObservationFactAuthority()
	if err != nil {
		t.Fatal(err)
	}
	revision, err := cognition.NewWorldRevision("visible-fact-invalid", 1, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"duplicate":    `{"format":"symbolic-observation.v1","format":"symbolic-observation.v1","predicates":[],"records":[],"goal_satisfied":false}`,
		"changed-hash": `{"format":"symbolic-observation.v1","predicates":[],"records":[{"id":"record-001","location":"stage-001","content":"changed","content_sha256":"` + strings.Repeat("b", 64) + `"}],"goal_satisfied":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			observation, err := cognition.NewObservation(
				cognition.ObservationID("visible-fact-invalid-"+name), revision, "symbolic_state", content,
			)
			if err != nil {
				t.Fatal(err)
			}
			transition := cognition.Transition{
				Current: revision, Observations: []cognition.Observation{observation}, Effects: []cognition.Effect{},
			}
			if _, err := authority.MapTransitionFacts(
				newVisibleFactLedger(t, 9).MaterializedState(), "", transition,
			); err == nil {
				t.Fatal("invalid public observation was accepted")
			}
		})
	}
}

func firstVisibleFactTransition(
	t *testing.T,
	fixture MicrogauntletCase,
	surface Surface,
	identity int,
) (cognition.Transition, cognition.RegisteredAction) {
	t.Helper()
	episode, err := cognition.NewEpisodeRef(cognition.EpisodeID("visible-fact-episode-" + string(rune('a'+identity))))
	if err != nil {
		t.Fatal(err)
	}
	actor := cognition.AttemptRef{
		JobID: int64(identity), Generation: 1, StepID: 1, Attempt: 1, WorkerID: "visible-fact-worker",
	}
	environment, closeEnvironment, err := newBenchmarkEnvironment(fixture, episode, actor, surface)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeEnvironment() })
	start, err := environment.Start(t.Context(), fixture.SealedEnvironmentScenario().Ref())
	if err != nil {
		t.Fatal(err)
	}
	witness := fixture.generated.PrivateOracle().Witness[0]
	schema, exists := fixture.SealedEnvironmentScenario().Catalog().Schema(witness.Request.Kind)
	if !exists {
		t.Fatal("witness schema disappeared")
	}
	action, err := cognition.NewRegisteredAction(witness.ID, actor, schema, witness.Request, []cognition.EvidenceRef{})
	if err != nil {
		t.Fatal(err)
	}
	transition, err := environment.Apply(t.Context(), episode, start.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	return transition, action
}

func newVisibleFactLedger(t *testing.T, jobID int64) *taskstate.Ledger {
	t.Helper()
	owner := taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: jobID, RunID: "01234567-89ab-cdef-0123-456789abcdef",
	}
	id, err := taskstate.NewLedgerID(owner)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := taskstate.NewLedger(id, owner)
	if err != nil {
		t.Fatal(err)
	}
	return ledger
}
