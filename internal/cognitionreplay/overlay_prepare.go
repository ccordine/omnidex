package cognitionreplay

import (
	"fmt"
	"sort"
)

type preparedOverlay struct {
	manifest PrivateOverlayManifest
	sources  []PrivateSource
	events   []PrivateEvent
	frames   []PrivateFrame
	chunked  []ChunkedBlobBinding
	blobs    []Blob
}

func preparePrivateOverlay(
	input PrivateOverlayInput,
	baseRaw []byte,
) (preparedOverlay, error) {
	base, err := VerifyBase(baseRaw)
	if err != nil {
		return preparedOverlay{}, fmt.Errorf("private overlay base replay: %w", err)
	}
	if err := requirePrivateOverlayTerminal(base.Manifest().TerminalAuthority); err != nil {
		return preparedOverlay{}, err
	}
	if input.BaseReplaySHA256 != base.SHA256() ||
		input.TerminalAuthoritySHA256 != base.Manifest().TerminalAuthoritySHA256 ||
		!validDigest(input.OracleSHA256) || !validDigest(input.EvaluationSHA256) {
		return preparedOverlay{}, fmt.Errorf("private overlay binding authority is invalid")
	}
	result := preparedOverlay{
		sources: clonePrivateSources(input.Sources), events: clonePrivateEvents(input.Events),
		frames:  clonePrivateFrames(input.Frames),
		chunked: cloneChunkedBlobBindings(input.ChunkedBlobs), blobs: cloneBlobs(input.Blobs),
	}
	if err := preparePrivateEventChain(result.events); err != nil {
		return preparedOverlay{}, err
	}
	if err := preparePrivateFrameChain(result.frames); err != nil {
		return preparedOverlay{}, err
	}
	sort.Slice(result.blobs, func(left, right int) bool {
		return result.blobs[left].SHA256 < result.blobs[right].SHA256
	})
	sort.Slice(result.chunked, func(left, right int) bool {
		return result.chunked[left].Manifest.SHA256 < result.chunked[right].Manifest.SHA256
	})
	sourceSHA, err := digestCanonical(result.sources)
	if err != nil {
		return preparedOverlay{}, err
	}
	eventSHA, err := digestCanonical(result.events)
	if err != nil {
		return preparedOverlay{}, err
	}
	frameSHA, err := digestCanonical(result.frames)
	if err != nil {
		return preparedOverlay{}, err
	}
	chunkedSHA, err := digestCanonical(result.chunked)
	if err != nil {
		return preparedOverlay{}, err
	}
	result.manifest = PrivateOverlayManifest{
		Schema: PrivateOverlaySchemaV2, Container: containerPrivate,
		BaseReplaySHA256:        input.BaseReplaySHA256,
		TerminalAuthoritySHA256: input.TerminalAuthoritySHA256,
		OracleSHA256:            input.OracleSHA256, EvaluationSHA256: input.EvaluationSHA256,
		SourceCount: len(result.sources), EventCount: len(result.events),
		FrameCount: len(result.frames), ChunkedBlobCount: len(result.chunked), BlobCount: len(result.blobs),
		SourceIndexSHA256: sourceSHA, EventIndexSHA256: eventSHA,
		FrameIndexSHA256: frameSHA, ChunkedBlobIndexSHA256: chunkedSHA,
		ChunkedBlobs: cloneChunkedBlobBindings(result.chunked), Entries: []ContainerEntry{},
	}
	if err := validatePreparedOverlay(
		result.manifest, result.sources, result.events, result.frames, result.chunked,
		blobsByDigest(result.blobs),
	); err != nil {
		return preparedOverlay{}, err
	}
	return result, nil
}

func requirePrivateOverlayTerminal(authority TerminalAuthority) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	if _, sealed := authority.SealedEpisode(); !sealed {
		return fmt.Errorf("private replay requires one sealed episode terminal authority")
	}
	return nil
}

func preparePrivateEventChain(values []PrivateEvent) error {
	previous := ""
	for index := range values {
		event := &values[index]
		if event.PreviousSHA256 != "" || event.EventSHA256 != "" {
			return fmt.Errorf("unprepared private event %d contains derived identity", index+1)
		}
		event.PreviousSHA256 = previous
		sha, err := privateEventDigest(*event)
		if err != nil {
			return err
		}
		event.EventSHA256 = sha
		previous = sha
	}
	return nil
}

func preparePrivateFrameChain(values []PrivateFrame) error {
	previous := ""
	for index := range values {
		frame := &values[index]
		if frame.PreviousSHA256 != "" || frame.FrameSHA256 != "" {
			return fmt.Errorf("unprepared private frame %d contains derived identity", index+1)
		}
		frame.PreviousSHA256 = previous
		sha, err := privateFrameDigest(*frame)
		if err != nil {
			return err
		}
		frame.FrameSHA256 = sha
		previous = sha
	}
	return nil
}

func privateEventDigest(value PrivateEvent) (string, error) {
	value.EventSHA256 = ""
	return digestCanonical(value)
}

func privateFrameDigest(value PrivateFrame) (string, error) {
	value.FrameSHA256 = ""
	return digestCanonical(value)
}
