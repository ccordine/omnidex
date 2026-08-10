package cognitiongauntlet

import (
	"context"
	"testing"
)

func TestLabyrinthRelevanceIsDerivedFromPrivateOracleAfterEpisodeSeal(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	oracleRun, err := RunOracleBaseline(
		context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	episode := modelEpisodeWithConsumerProjection(t, fixture, oracleRun)
	relevance, err := BuildLabyrinthProjectionRelevance(
		fixture, episode, oracleRun.Authority.SurfaceVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	if relevance.EpisodeSealSHA256 != episode.SealSHA256 || len(relevance.RelevantRefs) != 1 ||
		len(relevance.CriticalUses) != 1 {
		t.Fatalf("private relevance join=%+v", relevance)
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := EvaluateCleanDesk(episode, oracle, relevance)
	if err != nil {
		t.Fatal(err)
	}
	if !metrics.ConcentrationQualified || metrics.MissingCriticalBytes != 0 {
		t.Fatalf("oracle-derived clean desk=%+v", metrics)
	}
	evaluation, causal, err := ScoreMicrogauntletEpisode(
		fixture, oracleRun.Authority.SurfaceVersion, episode,
		SymbolicEvaluationEvidence{
			GoalPredicateSatisfied: true, ValidTerminalState: true,
			ActualDecisionCost: oracle.WitnessCost,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.CleanDesk == nil || !evaluation.CleanDesk.ConcentrationQualified ||
		causal.AcquiredEvidence != causal.RequiredEvidence {
		t.Fatalf("microgauntlet evaluation=%+v causal=%+v", evaluation, causal)
	}
	callerLabels := SymbolicEvaluationEvidence{
		ProjectionRelevance: &relevance,
	}
	if _, _, err := ScoreMicrogauntletEpisode(
		fixture, oracleRun.Authority.SurfaceVersion, episode, callerLabels,
	); err == nil {
		t.Fatal("microgauntlet scorer accepted caller-authored relevance labels")
	}
}

func modelEpisodeWithConsumerProjection(
	t *testing.T,
	fixture MicrogauntletCase,
	run OracleBaselineResult,
) SealedEpisode {
	t.Helper()
	manifest := run.Episode.Manifest
	var err error
	manifest.Trace, err = cloneTrace(run.Episode.Manifest.Trace)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Variant = VariantFullCognition
	oracle := fixture.generated.PrivateOracle()
	acquisition := oracle.EvidenceUses[0].AcquisitionActionID
	consumer := oracle.EvidenceUses[0].RequiredByActionID
	observations, err := acquisitionObservations(run.Episode)
	if err != nil {
		t.Fatal(err)
	}
	observation, exists := observations[acquisition]
	if !exists {
		t.Fatal("oracle trace lacks acquisition observation")
	}
	ref := observationProjectionRef(observation)
	projection := ProjectionTrace{
		Schema: ProjectionTraceSchemaV1, ProjectionID: "projection-consumer",
		ProjectionSHA256: run.Episode.SealSHA256, TokenEstimator: "utf8-bytes-div-four.v1",
		Selected: []ProjectedReference{{
			Ref: ref, SourceRefs: []ProjectionReferenceIdentity{},
			RenderedBytes: int64(len([]byte(observation.Content))),
		}},
	}
	projection.RenderedBytes = projection.Selected[0].RenderedBytes + 32
	projection.EstimatedTokens = (projection.RenderedBytes + 3) / 4
	inputBytes := projection.RenderedBytes + 128
	call := ModelCallTrace{
		Schema: ModelCallTraceSchemaV1, ProjectionID: projection.ProjectionID,
		ProjectionSHA256: projection.ProjectionSHA256, Budget: manifest.StationBudget,
		InputBytes: inputBytes, InputTokens: (inputBytes + 3) / 4,
		OutputBytes: 64, OutputTokens: 16,
	}
	insertAt := -1
	for index, entry := range manifest.Trace {
		if entry.Kind == TraceAction && entry.ID == string(consumer) {
			insertAt = index
			break
		}
	}
	if insertAt < 0 {
		t.Fatal("oracle trace lacks evidence consumer action")
	}
	projectionEntry := TraceEntry{
		Kind: TraceProjection, ID: projection.ProjectionID,
		Revision: manifest.Trace[insertAt].Revision, Payload: mustTraceJSONObject(t, projection),
	}
	callEntry := TraceEntry{
		Kind: TraceModelCall, ID: "model-call-consumer",
		Revision: manifest.Trace[insertAt].Revision, Payload: mustTraceJSONObject(t, call),
	}
	trace := make([]TraceEntry, 0, len(manifest.Trace)+2)
	trace = append(trace, manifest.Trace[:insertAt]...)
	trace = append(trace, projectionEntry, callEntry)
	trace = append(trace, manifest.Trace[insertAt:]...)
	for index := range trace {
		trace[index].Sequence = uint64(index + 1)
	}
	manifest.Trace = trace
	manifest.TraceSHA256 = ""
	manifest.Resources.ModelCalls = 1
	manifest.Resources.ModelDecisions = 1
	manifest.Resources.ContextBytes = inputBytes
	manifest.Resources.PeakContextBytes = inputBytes
	manifest.Resources.InputTokens = call.InputTokens
	manifest.Resources.OutputBytes = call.OutputBytes
	manifest.Resources.OutputTokens = call.OutputTokens
	prepared, err := prepareEpisodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sealed := SealedEpisode{Schema: EpisodeSealSchemaV1, Manifest: prepared}
	sealed.SealSHA256, err = digestJSON(prepared)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
