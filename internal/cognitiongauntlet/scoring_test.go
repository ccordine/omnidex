package cognitiongauntlet

import (
	"context"
	"strings"
	"testing"
)

func TestSymbolicScoringUsesSealedEpisodeAndPrivateOracleWithoutLLMJudge(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunOracleBaseline(context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1))
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := ScoreSealedEpisode(run.Episode, oracle, SymbolicEvaluationEvidence{
		GoalPredicateSatisfied: true, ValidTerminalState: true,
		ActualDecisionCost: oracle.WitnessCost,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.GoalSuccess || evaluation.Attribution != nil {
		t.Fatalf("evaluation=%+v", evaluation)
	}

	changed := oracle
	changed.PublicSHA256 = strings.Repeat("f", 64)
	if _, err := ScoreSealedEpisode(run.Episode, changed, SymbolicEvaluationEvidence{}); err == nil {
		t.Fatal("symbolic scorer accepted an oracle for another public scenario")
	}
	if _, err := ScoreSealedEpisode(run.Episode, oracle, SymbolicEvaluationEvidence{
		ValidTerminalState: true,
	}); err == nil {
		t.Fatal("symbolic scorer accepted a model-visible success rejected by world truth")
	}
}

func TestSymbolicScoringAttributesUnrecognizedWorldCompletion(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunOracleBaseline(context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1))
	if err != nil {
		t.Fatal(err)
	}
	partialManifest := run.Episode.Manifest
	partialManifest.Outcome = Outcome{PublicOutcome: "runtime stopped before recognizing completion"}
	partialManifest.TraceSHA256 = ""
	prepared, err := prepareEpisodeManifest(partialManifest)
	if err != nil {
		t.Fatal(err)
	}
	partial := SealedEpisode{Schema: EpisodeSealSchemaV1, Manifest: prepared}
	partial.SealSHA256, err = digestJSON(partial.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := ScoreSealedEpisode(partial, oracle, SymbolicEvaluationEvidence{
		GoalPredicateSatisfied: true, ValidTerminalState: true,
		ActualDecisionCost:   oracle.WitnessCost,
		Failure:              FailureTrace{CompletionCheckID: "completion-check-final"},
		PrivateAuthorityRefs: []string{"completion-check-final"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.GoalSuccess || evaluation.Attribution == nil ||
		evaluation.Attribution.Class != FailureCompletion {
		t.Fatalf("evaluation=%+v", evaluation)
	}
}

func TestSymbolicScoringRejectsFailureAttributionWithoutSealedProof(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunOracleBaseline(context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1))
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ScoreSealedEpisode(run.Episode, oracle, SymbolicEvaluationEvidence{
		ActualDecisionCost: oracle.WitnessCost,
		Failure: FailureTrace{
			NecessaryEvidence: true, RequiredEvidenceID: "unsealed-required-evidence",
		},
	}); err == nil {
		t.Fatal("symbolic scorer accepted failure attribution without sealed proof authority")
	}
}
