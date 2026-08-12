package cognitionreplay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublicBaseBindsOnlyPublicChunkedBlobs(t *testing.T) {
	path := writeChunkFixture(t, "public")
	public, blobs, err := ChunkPublicFile(path, "public-evidence", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	input := baseInputWithChunkedBlob(t, public, blobs)
	artifact, err := ExportStructuralBase(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBase(artifact.Bytes); err != nil {
		t.Fatal(err)
	}

	private, privateBlobs, err := ChunkPrivateFile(
		path, "private-evidence", "application/octet-stream",
	)
	if err != nil {
		t.Fatal(err)
	}
	input = baseInputWithChunkedBlob(t, private, privateBlobs)
	if _, err := ExportStructuralBase(input); err == nil {
		t.Fatal("public base accepted a private-world chunk manifest")
	}
}

func TestPrivateOverlayBindsOnlyPrivateChunkedBlobs(t *testing.T) {
	base, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	path := writeChunkFixture(t, "private")
	private, blobs, err := ChunkPrivateFile(path, "private-world", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	input := privateInputWithChunkedBlob(t, base, private, blobs)
	overlay, err := ExportPrivateOverlay(input, base.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPrivateOverlay(overlay.Bytes, base.Bytes); err != nil {
		t.Fatal(err)
	}

	public, publicBlobs, err := ChunkPublicFile(
		path, "public-world", "application/octet-stream",
	)
	if err != nil {
		t.Fatal(err)
	}
	input = privateInputWithChunkedBlob(t, base, public, publicBlobs)
	if _, err := ExportPrivateOverlay(input, base.Bytes); err == nil {
		t.Fatal("private overlay accepted a public-agent-knowledge chunk manifest")
	}
}

func TestContainerRejectsDuplicateChunkManifestBinding(t *testing.T) {
	path := writeChunkFixture(t, "duplicate")
	binding, blobs, err := ChunkPublicFile(path, "evidence", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	input := baseInputWithChunkedBlob(t, binding, blobs)
	input.ChunkedBlobs = append(input.ChunkedBlobs, binding)
	if _, err := ExportStructuralBase(input); err == nil {
		t.Fatal("duplicate chunk manifest binding was accepted")
	}
}

func baseInputWithChunkedBlob(
	t *testing.T,
	binding ChunkedBlobBinding,
	blobs []Blob,
) BaseInput {
	t.Helper()
	input := validBaseInput(t)
	source := SourceRecord{
		Ordinal: 4, CallOrdinal: 2, Phase: 1, Sequence: 1,
		Kind: "provider_evidence", ID: "provider-evidence", Payload: binding.Manifest,
	}
	input.Sources = append(input.Sources, source)
	input.Events = append(input.Events, Event{
		Sequence: 4, Kind: EventEvidenceAcquired, MappingSchema: StructuralMappingSchemaV1,
		Sources: []SourceRef{source.Ref()}, Payload: binding.Manifest,
	})
	input.Checkpoints[1].AfterEvent = 4
	input.Checkpoints[1].Delta.ThroughEvent = 4
	input.ChunkedBlobs = []ChunkedBlobBinding{binding}
	input.Blobs = append(input.Blobs, blobs...)
	return input
}

func privateInputWithChunkedBlob(
	t *testing.T,
	base Artifact,
	binding ChunkedBlobBinding,
	blobs []Blob,
) PrivateOverlayInput {
	t.Helper()
	input := validPrivateOverlayInput(t, base)
	source := PrivateSource{
		Ordinal: 3, Kind: PrivateSourceWorld, ID: "chunked-world", Payload: binding.Manifest,
	}
	input.Sources = append(input.Sources, source)
	input.Events = append(input.Events, PrivateEvent{
		Sequence: 3, Kind: PrivateEventWorldTruth,
		Sources: []PrivateSourceRef{source.Ref()}, Payload: binding.Manifest,
	})
	input.Frames[0].AfterEvent = 3
	input.ChunkedBlobs = []ChunkedBlobBinding{binding}
	input.Blobs = append(input.Blobs, blobs...)
	return input
}

func writeChunkFixture(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chunk.bin")
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
