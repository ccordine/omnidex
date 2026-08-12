package cognitionreplay

import (
	"bytes"
	"fmt"
)

func ExportPrivateOverlay(input PrivateOverlayInput, baseRaw []byte) (Artifact, error) {
	prepared, err := preparePrivateOverlay(input, baseRaw)
	if err != nil {
		return Artifact{}, err
	}
	sourceFiles, sourceEntries, err := buildPages(
		"private/sources", entryPrivateSource, prepared.sources,
		func(value PrivateSource) uint64 { return value.Ordinal },
	)
	if err != nil {
		return Artifact{}, err
	}
	eventFiles, eventEntries, err := buildPages(
		"private/events", entryPrivateEvent, prepared.events,
		func(value PrivateEvent) uint64 { return value.Sequence },
	)
	if err != nil {
		return Artifact{}, err
	}
	frameFiles, frameEntries, err := buildPages(
		"private/frames", entryPrivateFrame, prepared.frames,
		func(value PrivateFrame) uint64 { return value.Sequence },
	)
	if err != nil {
		return Artifact{}, err
	}
	blobFiles, blobEntries := buildBlobFiles(prepared.blobs)
	prepared.manifest.Entries = append(prepared.manifest.Entries, sourceEntries...)
	prepared.manifest.Entries = append(prepared.manifest.Entries, eventEntries...)
	prepared.manifest.Entries = append(prepared.manifest.Entries, frameEntries...)
	prepared.manifest.Entries = append(prepared.manifest.Entries, blobEntries...)
	manifestRaw, err := marshalCanonical(prepared.manifest)
	if err != nil {
		return Artifact{}, err
	}
	files := []containerFile{{name: manifestPath, body: manifestRaw}}
	files = append(files, sourceFiles...)
	files = append(files, eventFiles...)
	files = append(files, frameFiles...)
	files = append(files, blobFiles...)
	raw, err := encodeContainer(files)
	if err != nil {
		return Artifact{}, err
	}
	verified, err := VerifyPrivateOverlay(raw, baseRaw)
	if err != nil {
		return Artifact{}, fmt.Errorf("verify exported private replay overlay: %w", err)
	}
	artifact := Artifact{Bytes: bytes.Clone(raw), SHA256: digestBytes(raw)}
	if verified.SHA256() != artifact.SHA256 {
		return Artifact{}, fmt.Errorf("private replay digest changed during verification")
	}
	return artifact, nil
}
