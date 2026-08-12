package cognitionreplay

import (
	"bytes"
	"fmt"
)

func ExportStructuralBase(input BaseInput) (Artifact, error) {
	prepared, err := prepareBase(input)
	if err != nil {
		return Artifact{}, err
	}
	return exportPreparedBase(prepared, "structural")
}

func ExportSemanticProjection(input SemanticProjectionInput) (Artifact, error) {
	if _, sealed := input.TerminalAuthority.SealedEpisode(); !sealed {
		return Artifact{}, fmt.Errorf("semantic replay requires one sealed episode terminal authority")
	}
	prepared, err := prepareSemanticProjection(input)
	if err != nil {
		return Artifact{}, err
	}
	return exportPreparedBase(prepared, "semantic")
}

// ExportAblationSemanticProjection creates an unqualified public replay. It
// rejects contaminated evidence; private oracle evidence requires a distinct
// private replay container and is never admitted to a public base.
func ExportAblationSemanticProjection(input AblationSemanticProjectionInput) (Artifact, error) {
	if _, sealed := input.TerminalAuthority.SealedEpisode(); !sealed {
		return Artifact{}, fmt.Errorf("ablation semantic replay requires one sealed episode")
	}
	prepared, err := prepareAblationSemanticProjection(input)
	if err != nil {
		return Artifact{}, err
	}
	return exportPreparedBase(prepared, "ablation semantic")
}

func exportPreparedBase(prepared preparedBase, label string) (Artifact, error) {
	sourceFiles, sourceEntries, err := buildPages(
		"sources", entrySourcePage, prepared.sources, func(value SourceRecord) uint64 { return value.Ordinal },
	)
	if err != nil {
		return Artifact{}, err
	}
	eventFiles, eventEntries, err := buildPages(
		"events", entryEventPage, prepared.events, func(value Event) uint64 { return value.Sequence },
	)
	if err != nil {
		return Artifact{}, err
	}
	checkpointFiles, checkpointEntries, err := buildPages(
		"checkpoints", entryCheckpointPage, prepared.checkpoints,
		func(value KnowledgeCheckpoint) uint64 { return value.Sequence },
	)
	if err != nil {
		return Artifact{}, err
	}
	blobFiles, blobEntries := buildBlobFiles(prepared.blobs)
	prepared.manifest.Entries = append(prepared.manifest.Entries, sourceEntries...)
	prepared.manifest.Entries = append(prepared.manifest.Entries, eventEntries...)
	prepared.manifest.Entries = append(prepared.manifest.Entries, checkpointEntries...)
	prepared.manifest.Entries = append(prepared.manifest.Entries, blobEntries...)
	manifestRaw, err := marshalCanonical(prepared.manifest)
	if err != nil {
		return Artifact{}, err
	}
	files := []containerFile{{name: manifestPath, body: manifestRaw}}
	files = append(files, sourceFiles...)
	files = append(files, eventFiles...)
	files = append(files, checkpointFiles...)
	files = append(files, blobFiles...)
	raw, err := encodeContainer(files)
	if err != nil {
		return Artifact{}, err
	}
	verified, err := VerifyBase(raw)
	if err != nil {
		return Artifact{}, fmt.Errorf("verify exported %s replay: %w", label, err)
	}
	artifact := Artifact{Bytes: bytes.Clone(raw), SHA256: digestBytes(raw)}
	if verified.SHA256() != artifact.SHA256 {
		return Artifact{}, fmt.Errorf("exported replay digest changed during verification")
	}
	return artifact, nil
}

func buildBlobFiles(values []Blob) ([]containerFile, []ContainerEntry) {
	files := make([]containerFile, len(values))
	entries := make([]ContainerEntry, len(values))
	for index, blob := range values {
		name := "blobs/sha256/" + blob.SHA256
		files[index] = containerFile{name: name, body: bytes.Clone(blob.Data)}
		entries[index] = ContainerEntry{
			Path: name, Kind: entryBlob, SHA256: blob.SHA256, ByteCount: int64(len(blob.Data)),
		}
	}
	return files, entries
}
