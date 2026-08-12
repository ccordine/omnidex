package cognitionreplay

import (
	"bytes"
	"fmt"
)

func VerifyPrivateOverlay(raw []byte, baseRaw []byte) (VerifiedPrivateOverlay, error) {
	base, err := VerifyBase(baseRaw)
	if err != nil {
		return VerifiedPrivateOverlay{}, fmt.Errorf("private overlay base replay: %w", err)
	}
	if err := requirePrivateOverlayTerminal(base.Manifest().TerminalAuthority); err != nil {
		return VerifiedPrivateOverlay{}, err
	}
	files, err := decodeContainer(raw)
	if err != nil {
		return VerifiedPrivateOverlay{}, err
	}
	var manifest PrivateOverlayManifest
	if err := decodeCanonical(files[0].body, &manifest, "private replay manifest"); err != nil {
		return VerifiedPrivateOverlay{}, err
	}
	if err := validatePrivateManifestHeader(manifest); err != nil {
		return VerifiedPrivateOverlay{}, err
	}
	if manifest.BaseReplaySHA256 != base.SHA256() ||
		manifest.TerminalAuthoritySHA256 != base.Manifest().TerminalAuthoritySHA256 {
		return VerifiedPrivateOverlay{}, fmt.Errorf("private replay overlay binds a different public base")
	}
	if err := validateContainerEntries(manifest.Entries, files[1:]); err != nil {
		return VerifiedPrivateOverlay{}, err
	}
	sources, events, frames, blobBodies, err := decodePrivateEntries(manifest, files[1:])
	if err != nil {
		return VerifiedPrivateOverlay{}, err
	}
	blobs, err := bindPrivateBlobMediaTypes(
		sources, events, frames, manifest.ChunkedBlobs, blobBodies,
	)
	if err != nil {
		return VerifiedPrivateOverlay{}, err
	}
	if err := validatePreparedOverlay(
		manifest, sources, events, frames, manifest.ChunkedBlobs, blobs,
	); err != nil {
		return VerifiedPrivateOverlay{}, err
	}
	return VerifiedPrivateOverlay{manifest: manifest, sha256: digestBytes(raw)}, nil
}

func decodePrivateEntries(
	manifest PrivateOverlayManifest,
	files []containerFile,
) ([]PrivateSource, []PrivateEvent, []PrivateFrame, map[string][]byte, error) {
	sourcePages := pageCount(manifest.SourceCount)
	eventPages := pageCount(manifest.EventCount)
	framePages := pageCount(manifest.FrameCount)
	if len(files) != sourcePages+eventPages+framePages+manifest.BlobCount {
		return nil, nil, nil, nil, fmt.Errorf("private replay container partition is invalid")
	}
	cursor := 0
	sources := make([]PrivateSource, 0, manifest.SourceCount)
	for page := 0; page < sourcePages; page++ {
		values, err := decodePage[PrivateSource](files[cursor], manifest.Entries[cursor],
			fmt.Sprintf("private/sources/page-%06d.jsonl", page), entryPrivateSource,
			func(value PrivateSource) uint64 { return value.Ordinal })
		if err != nil {
			return nil, nil, nil, nil, err
		}
		sources = append(sources, values...)
		cursor++
	}
	events := make([]PrivateEvent, 0, manifest.EventCount)
	for page := 0; page < eventPages; page++ {
		values, err := decodePage[PrivateEvent](files[cursor], manifest.Entries[cursor],
			fmt.Sprintf("private/events/page-%06d.jsonl", page), entryPrivateEvent,
			func(value PrivateEvent) uint64 { return value.Sequence })
		if err != nil {
			return nil, nil, nil, nil, err
		}
		events = append(events, values...)
		cursor++
	}
	frames := make([]PrivateFrame, 0, manifest.FrameCount)
	for page := 0; page < framePages; page++ {
		values, err := decodePage[PrivateFrame](files[cursor], manifest.Entries[cursor],
			fmt.Sprintf("private/frames/page-%06d.jsonl", page), entryPrivateFrame,
			func(value PrivateFrame) uint64 { return value.Sequence })
		if err != nil {
			return nil, nil, nil, nil, err
		}
		frames = append(frames, values...)
		cursor++
	}
	blobs := make(map[string][]byte, manifest.BlobCount)
	previous := ""
	for ; cursor < len(files); cursor++ {
		entry, file := manifest.Entries[cursor], files[cursor]
		if entry.Kind != entryBlob || entry.First != 0 || entry.Last != 0 || entry.RecordCount != 0 ||
			file.name != "blobs/sha256/"+entry.SHA256 || (previous != "" && entry.SHA256 <= previous) {
			return nil, nil, nil, nil, fmt.Errorf("private replay blob entries are invalid or reordered")
		}
		blobs[entry.SHA256] = bytes.Clone(file.body)
		previous = entry.SHA256
	}
	if len(sources) != manifest.SourceCount || len(events) != manifest.EventCount ||
		len(frames) != manifest.FrameCount || len(blobs) != manifest.BlobCount {
		return nil, nil, nil, nil, fmt.Errorf("private replay content is incomplete")
	}
	return sources, events, frames, blobs, nil
}
