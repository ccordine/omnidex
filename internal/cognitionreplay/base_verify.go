package cognitionreplay

import (
	"bytes"
	"fmt"
)

func VerifyBase(raw []byte) (VerifiedBase, error) {
	files, err := decodeContainer(raw)
	if err != nil {
		return VerifiedBase{}, err
	}
	var manifest BaseManifest
	if err := decodeCanonical(files[0].body, &manifest, "public replay manifest"); err != nil {
		return VerifiedBase{}, err
	}
	if err := validateBaseManifestHeader(manifest); err != nil {
		return VerifiedBase{}, err
	}
	if err := validateContainerEntries(manifest.Entries, files[1:]); err != nil {
		return VerifiedBase{}, err
	}
	sources, events, checkpoints, blobBodies, err := decodeBaseEntries(manifest, files[1:])
	if err != nil {
		return VerifiedBase{}, err
	}
	blobs, err := bindBlobMediaTypes(
		manifest, sources, events, checkpoints, manifest.ChunkedBlobs, blobBodies,
	)
	if err != nil {
		return VerifiedBase{}, err
	}
	if err := validatePreparedBase(
		manifest, sources, events, checkpoints, manifest.ChunkedBlobs, blobs,
	); err != nil {
		return VerifiedBase{}, err
	}
	return VerifiedBase{
		manifest: cloneBaseManifest(manifest), sha256: digestBytes(raw),
		sources: cloneSourceRecords(sources), events: cloneEvents(events),
		checkpoints: cloneKnowledgeCheckpoints(checkpoints), blobs: cloneBlobMap(blobs),
	}, nil
}

func cloneBlobMap(values map[string]Blob) map[string]Blob {
	result := make(map[string]Blob, len(values))
	for digest, blob := range values {
		blob.Data = bytes.Clone(blob.Data)
		result[digest] = blob
	}
	return result
}

func validateContainerEntries(entries []ContainerEntry, files []containerFile) error {
	if len(entries) != len(files) {
		return fmt.Errorf("replay manifest entry count differs from its container")
	}
	for index, entry := range entries {
		file := files[index]
		if entry.Path != file.name || !validDigest(entry.SHA256) ||
			entry.SHA256 != digestBytes(file.body) || entry.ByteCount != int64(len(file.body)) ||
			entry.ByteCount <= 0 {
			return fmt.Errorf("replay manifest entry %d changed", index+1)
		}
	}
	return nil
}

func decodeBaseEntries(
	manifest BaseManifest,
	files []containerFile,
) ([]SourceRecord, []Event, []KnowledgeCheckpoint, map[string][]byte, error) {
	sourcePages := pageCount(manifest.SourceCount)
	eventPages := pageCount(manifest.EventCount)
	checkpointPages := pageCount(manifest.CheckpointCount)
	if len(files) != sourcePages+eventPages+checkpointPages+manifest.BlobCount {
		return nil, nil, nil, nil, fmt.Errorf("public replay container partition is invalid")
	}
	cursor := 0
	sources := make([]SourceRecord, 0, manifest.SourceCount)
	for page := 0; page < sourcePages; page++ {
		values, err := decodePage[SourceRecord](files[cursor], manifest.Entries[cursor],
			fmt.Sprintf("sources/page-%06d.jsonl", page), entrySourcePage,
			func(value SourceRecord) uint64 { return value.Ordinal })
		if err != nil {
			return nil, nil, nil, nil, err
		}
		sources = append(sources, values...)
		cursor++
	}
	events := make([]Event, 0, manifest.EventCount)
	for page := 0; page < eventPages; page++ {
		values, err := decodePage[Event](files[cursor], manifest.Entries[cursor],
			fmt.Sprintf("events/page-%06d.jsonl", page), entryEventPage,
			func(value Event) uint64 { return value.Sequence })
		if err != nil {
			return nil, nil, nil, nil, err
		}
		events = append(events, values...)
		cursor++
	}
	checkpoints := make([]KnowledgeCheckpoint, 0, manifest.CheckpointCount)
	for page := 0; page < checkpointPages; page++ {
		values, err := decodePage[KnowledgeCheckpoint](files[cursor], manifest.Entries[cursor],
			fmt.Sprintf("checkpoints/page-%06d.jsonl", page), entryCheckpointPage,
			func(value KnowledgeCheckpoint) uint64 { return value.Sequence })
		if err != nil {
			return nil, nil, nil, nil, err
		}
		checkpoints = append(checkpoints, values...)
		cursor++
	}
	blobs := make(map[string][]byte, manifest.BlobCount)
	previous := ""
	for ; cursor < len(files); cursor++ {
		entry, file := manifest.Entries[cursor], files[cursor]
		if entry.Kind != entryBlob || entry.First != 0 || entry.Last != 0 || entry.RecordCount != 0 ||
			file.name != "blobs/sha256/"+entry.SHA256 || (previous != "" && entry.SHA256 <= previous) {
			return nil, nil, nil, nil, fmt.Errorf("public replay blob entries are invalid or reordered")
		}
		blobs[entry.SHA256] = bytes.Clone(file.body)
		previous = entry.SHA256
	}
	if len(sources) != manifest.SourceCount || len(events) != manifest.EventCount ||
		len(checkpoints) != manifest.CheckpointCount || len(blobs) != manifest.BlobCount {
		return nil, nil, nil, nil, fmt.Errorf("public replay content is incomplete")
	}
	return sources, events, checkpoints, blobs, nil
}

func decodePage[T any](
	file containerFile,
	entry ContainerEntry,
	wantPath string,
	wantKind EntryKind,
	sequence func(T) uint64,
) ([]T, error) {
	if file.name != wantPath || entry.Path != wantPath || entry.Kind != wantKind ||
		entry.RecordCount <= 0 || entry.RecordCount > maxPageItems {
		return nil, fmt.Errorf("replay page %q authority is invalid", wantPath)
	}
	values, err := decodeJSONLines[T](file.body, string(wantKind))
	if err != nil {
		return nil, err
	}
	if len(values) != entry.RecordCount || sequence(values[0]) != entry.First ||
		sequence(values[len(values)-1]) != entry.Last {
		return nil, fmt.Errorf("replay page %q range changed", wantPath)
	}
	return values, nil
}

func pageCount(records int) int {
	if records <= 0 {
		return 0
	}
	return (records + maxPageItems - 1) / maxPageItems
}
