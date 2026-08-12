package cognitionreplay

import "fmt"

func validatePreparedOverlay(
	manifest PrivateOverlayManifest,
	sources []PrivateSource,
	events []PrivateEvent,
	frames []PrivateFrame,
	chunked []ChunkedBlobBinding,
	blobs map[string]Blob,
) error {
	if err := validatePrivateManifestHeader(manifest); err != nil {
		return err
	}
	if len(sources) == 0 || len(sources) > maxSources || len(events) == 0 ||
		len(events) > maxEvents || len(frames) == 0 || len(frames) > maxCheckpoints ||
		len(blobs) == 0 || len(blobs) > maxBlobs || manifest.SourceCount != len(sources) ||
		manifest.EventCount != len(events) || manifest.FrameCount != len(frames) ||
		manifest.ChunkedBlobCount != len(chunked) || manifest.BlobCount != len(blobs) {
		return fmt.Errorf("private replay record or blob count is invalid")
	}
	if err := validateChunkedBlobBindings(
		chunked, manifest.ChunkedBlobs, ChunkedBlobPrivateWorld,
	); err != nil {
		return err
	}
	if err := validatePrivateSources(sources); err != nil {
		return err
	}
	if err := validatePrivateArtifactBindings(manifest, sources); err != nil {
		return err
	}
	if err := validatePrivateEvents(events, sources); err != nil {
		return err
	}
	if err := validatePrivateFrames(frames, len(events)); err != nil {
		return err
	}
	if err := validatePrivateIndexes(manifest, sources, events, frames, chunked); err != nil {
		return err
	}
	return validatePrivateBlobClosure(sources, events, frames, chunked, blobs)
}

func validatePrivateManifestHeader(manifest PrivateOverlayManifest) error {
	if manifest.Schema != PrivateOverlaySchemaV2 || manifest.Container != containerPrivate ||
		!validDigest(manifest.BaseReplaySHA256) || !validDigest(manifest.TerminalAuthoritySHA256) ||
		!validDigest(manifest.OracleSHA256) || !validDigest(manifest.EvaluationSHA256) ||
		!validDigest(manifest.SourceIndexSHA256) || !validDigest(manifest.EventIndexSHA256) ||
		!validDigest(manifest.FrameIndexSHA256) || !validDigest(manifest.ChunkedBlobIndexSHA256) ||
		manifest.ChunkedBlobs == nil || manifest.Entries == nil {
		return fmt.Errorf("private replay manifest authority is invalid")
	}
	return nil
}

func validatePrivateSources(values []PrivateSource) error {
	seen := make(map[string]struct{}, len(values))
	for index, source := range values {
		if source.Ordinal != uint64(index+1) || source.Validate() != nil {
			return fmt.Errorf("private replay source %d is invalid", index+1)
		}
		key := string(source.Kind) + "\x00" + source.ID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("private replay source identity is duplicated")
		}
		if index > 0 && !privateSourceLess(values[index-1], source) {
			return fmt.Errorf("private replay sources are reordered")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validatePrivateArtifactBindings(manifest PrivateOverlayManifest, values []PrivateSource) error {
	oracles, evaluations := 0, 0
	for _, source := range values {
		switch source.Kind {
		case PrivateSourceOracle:
			if source.Payload.SHA256 != manifest.OracleSHA256 {
				return fmt.Errorf("private replay oracle source changed")
			}
			oracles++
		case PrivateSourceEvaluation:
			if source.Payload.SHA256 != manifest.EvaluationSHA256 {
				return fmt.Errorf("private replay evaluation source changed")
			}
			evaluations++
		}
	}
	if oracles != 1 || evaluations != 1 {
		return fmt.Errorf("private replay requires one exact oracle and evaluation source")
	}
	return nil
}

func privateSourceLess(left, right PrivateSource) bool {
	leftRank, rightRank := privateSourceRank(left.Kind), privateSourceRank(right.Kind)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	return left.ID < right.ID
}

func privateSourceRank(value PrivateSourceKind) int {
	switch value {
	case PrivateSourceOracle:
		return 1
	case PrivateSourceEvaluation:
		return 2
	case PrivateSourceWorld:
		return 3
	default:
		return 0
	}
}

func validatePrivateEvents(values []PrivateEvent, sources []PrivateSource) error {
	byOrdinal := make(map[uint64]PrivateSourceRef, len(sources))
	cited := make(map[uint64]struct{}, len(sources))
	worldTruthEvents, evaluationEvents := 0, 0
	for _, source := range sources {
		byOrdinal[source.Ordinal] = source.Ref()
	}
	previous := ""
	for index, event := range values {
		if event.Sequence != uint64(index+1) || !validPrivateEventKind(event.Kind) ||
			event.Payload.Validate() != nil || len(event.Sources) == 0 ||
			event.PreviousSHA256 != previous || !validDigest(event.EventSHA256) {
			return fmt.Errorf("private replay event %d authority is invalid", index+1)
		}
		if !validPrivateEventSources(event.Kind, event.Sources) {
			return fmt.Errorf("private replay event %d source class is invalid", index+1)
		}
		if event.Kind == PrivateEventWorldTruth {
			worldTruthEvents++
		} else {
			evaluationEvents++
		}
		for sourceIndex, ref := range event.Sources {
			if ref.Validate() != nil || byOrdinal[ref.Ordinal] != ref ||
				(sourceIndex > 0 && ref.Ordinal <= event.Sources[sourceIndex-1].Ordinal) {
				return fmt.Errorf("private replay event %d source binding is invalid", index+1)
			}
			cited[ref.Ordinal] = struct{}{}
		}
		want, err := privateEventDigest(event)
		if err != nil || want != event.EventSHA256 {
			return fmt.Errorf("private replay event %d hash changed", index+1)
		}
		previous = event.EventSHA256
	}
	if len(cited) != len(sources) {
		return fmt.Errorf("private replay contains an orphan source")
	}
	if worldTruthEvents == 0 || evaluationEvents == 0 {
		return fmt.Errorf("private replay requires distinct world-truth and evaluation events")
	}
	return nil
}

func validPrivateEventSources(kind PrivateEventKind, sources []PrivateSourceRef) bool {
	want := PrivateSourceEvaluation
	if kind == PrivateEventWorldTruth {
		want = PrivateSourceOracle
	}
	for _, source := range sources {
		if source.Kind != want && !(kind == PrivateEventWorldTruth && source.Kind == PrivateSourceWorld) {
			return false
		}
	}
	return true
}

func validatePrivateFrames(values []PrivateFrame, eventCount int) error {
	previousHash := ""
	previousEvent := uint64(0)
	for index, frame := range values {
		if frame.Sequence != uint64(index+1) || frame.AfterEvent <= previousEvent ||
			frame.AfterEvent > uint64(eventCount) || frame.AfterEvent-previousEvent > maxCheckpointInterval ||
			frame.PreviousSHA256 != previousHash || frame.Snapshot.Validate() != nil ||
			!validDigest(frame.FrameSHA256) {
			return fmt.Errorf("private replay frame %d authority is invalid", index+1)
		}
		if index == 0 && frame.Delta != nil {
			return fmt.Errorf("first private replay frame cannot claim a prior-state delta")
		}
		if index > 0 && (frame.Delta == nil || frame.Delta.Validate() != nil) {
			return fmt.Errorf("private replay frame %d lacks a bounded delta", index+1)
		}
		want, err := privateFrameDigest(frame)
		if err != nil || want != frame.FrameSHA256 {
			return fmt.Errorf("private replay frame %d hash changed", index+1)
		}
		previousHash, previousEvent = frame.FrameSHA256, frame.AfterEvent
	}
	if previousEvent != uint64(eventCount) {
		return fmt.Errorf("private replay frames do not cover every truth event")
	}
	return nil
}

func validatePrivateIndexes(
	manifest PrivateOverlayManifest,
	sources []PrivateSource,
	events []PrivateEvent,
	frames []PrivateFrame,
	chunked []ChunkedBlobBinding,
) error {
	sourceSHA, sourceErr := digestCanonical(sources)
	eventSHA, eventErr := digestCanonical(events)
	frameSHA, frameErr := digestCanonical(frames)
	chunkedSHA, chunkedErr := digestCanonical(chunked)
	if sourceErr != nil || eventErr != nil || frameErr != nil ||
		chunkedErr != nil ||
		manifest.SourceIndexSHA256 != sourceSHA || manifest.EventIndexSHA256 != eventSHA ||
		manifest.FrameIndexSHA256 != frameSHA || manifest.ChunkedBlobIndexSHA256 != chunkedSHA {
		return fmt.Errorf("private replay ordered index digest changed")
	}
	return nil
}
