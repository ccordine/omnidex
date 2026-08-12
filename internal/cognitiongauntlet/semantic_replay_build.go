package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func buildProductionSemanticReplay(
	bundle PublicInferenceBundle,
	episode SealedEpisode,
	trace productionTrace,
	supplement semanticReplaySupplement,
) (cognitionreplay.SemanticProjectionInput, error) {
	terminal, err := semanticReplayTerminal(trace, episode)
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	sources, blobs, err := semanticReplaySources(trace.Records)
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	frozen, err := bundle.Authority.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	runtimeBudget, err := bundle.Authority.Budget.RuntimeBudget()
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	state := newSemanticReplayState(
		trace, sources, blobs, frozen, bundle.Goal, bundle.Completion, bundle.Catalog,
		runtimeBudget, supplement,
	)
	for index, record := range trace.Records {
		if err := state.accept(index, record); err != nil {
			return cognitionreplay.SemanticProjectionInput{}, err
		}
	}
	events, checkpoints, eventBlobs, err := state.finish(episode.Manifest.Outcome)
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	blobs = append(blobs, eventBlobs...)
	publicBundle, publicChunked, publicBlobs, err := semanticReplayProjectionContent(
		"public-inference-bundle", bundle,
	)
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	sealedEpisode, episodeChunked, episodeBlobs, err := semanticReplayProjectionContent(
		"sealed-episode", episode,
	)
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	traceHeader, traceChunked, traceBlobs, err := semanticReplayProjectionContent(
		"production-trace-header", semanticReplayHeader(trace.Header),
	)
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	blobs = append(blobs, publicBlobs...)
	blobs = append(blobs, episodeBlobs...)
	blobs = append(blobs, traceBlobs...)
	blobs = append(blobs, supplement.blobs...)
	blobs, err = uniqueReplayBlobs(blobs)
	if err != nil {
		return cognitionreplay.SemanticProjectionInput{}, err
	}
	chunked := append(publicChunked, episodeChunked...)
	chunked = append(chunked, traceChunked...)
	chunked = append(chunked, supplement.chunked...)
	return cognitionreplay.SemanticProjectionInput{
		TerminalAuthority:     terminal,
		PublicWorldSHA256:     bundle.Authority.Scenario.SHA256,
		PublicWorldSchema:     semanticReplayPublicWorldSchema,
		PublicAuthoritySHA256: publicSHA,
		PublicBundleAuthority: publicBundle, SealedEpisodeAuthority: sealedEpisode,
		ProductionTraceAuthority: traceHeader,
		Sidecars:                 append([]cognitionreplay.ProjectionSidecarAuthority(nil), supplement.sidecars...),
		Sources:                  sources, Events: events, Checkpoints: checkpoints,
		ChunkedBlobs: chunked, Blobs: blobs,
	}, nil
}

func semanticReplaySources(records []queue.CognitionSealedTraceRecord) (
	[]cognitionreplay.SourceRecord,
	[]cognitionreplay.Blob,
	error,
) {
	if err := queue.VerifyCognitionSealedTraceRecordOrder(records); err != nil {
		return nil, nil, fmt.Errorf("sealed cognition source order: %w", err)
	}
	sources := make([]cognitionreplay.SourceRecord, len(records))
	blobs := make([]cognitionreplay.Blob, 0, len(records))
	for index, record := range records {
		if len(record.Payload) == 0 || len(record.Payload) > cognitionreplay.MaxDirectBlobBytes {
			return nil, nil, fmt.Errorf(
				"sealed cognition source %q has invalid direct payload size %d",
				record.ID, len(record.Payload),
			)
		}
		blob, err := cognitionreplay.NewBlob("application/json", record.Payload)
		if err != nil || blob.SHA256 != record.SHA256 {
			return nil, nil, fmt.Errorf("sealed cognition source %q payload changed: %v", record.ID, err)
		}
		source := cognitionreplay.SourceRecord{
			Ordinal: uint64(index + 1), CallOrdinal: record.CallOrdinal,
			Phase: record.Phase, Sequence: record.Sequence, Kind: record.Kind,
			ID: record.ID, Payload: blob.Ref(),
		}
		if source.Validate() != nil {
			return nil, nil, fmt.Errorf("sealed cognition source order or authority changed at %d", index+1)
		}
		sources[index] = source
		blobs = append(blobs, blob)
	}
	return sources, blobs, nil
}

func validateSemanticReplayOutcome(status queue.CognitionEpisodeStatus, outcome Outcome) error {
	wantSatisfied := status == queue.CognitionEpisodeCompleted
	if (status != queue.CognitionEpisodeCompleted && status != queue.CognitionEpisodeFailed &&
		status != queue.CognitionEpisodeCanceled) || !outcome.Terminal ||
		outcome.GoalSatisfied != wantSatisfied {
		return fmt.Errorf("semantic replay terminal outcome differs from queue authority")
	}
	if (status == queue.CognitionEpisodeCompleted && outcome.FailureCode != "") ||
		(status != queue.CognitionEpisodeCompleted && outcome.FailureCode == "") {
		return fmt.Errorf("semantic replay terminal failure disposition changed")
	}
	return nil
}
