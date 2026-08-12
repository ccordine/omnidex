package cognitionreplay

import (
	"bytes"
	"testing"
)

func TestSemanticProjectionReassemblesLargeAuthorityWithoutFallback(t *testing.T) {
	input := semanticProjectionContentFixture(t)
	artifact, err := ExportSemanticProjection(input)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyBase(artifact.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	authority := verified.Manifest().ProjectionAuthority
	if authority == nil || authority.SealedEpisode.Storage != ProjectionContentChunked ||
		len(verified.Manifest().ChunkedBlobs) != 1 {
		t.Fatalf("large projection authority was not one exact chunked value: %+v", authority)
	}
	raw, err := verified.ProjectionContent(authority.SealedEpisode)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= MaxDirectBlobBytes || !bytes.Equal(raw, projectionLargeJSON()) {
		t.Fatal("large projection authority did not reassemble byte-exactly")
	}
	if _, err := verified.ProjectionContent(ProjectionContentAuthority{}); err == nil {
		t.Fatal("invalid projection content acquired an implicit fallback")
	}
}

func TestProjectionContentUsesOneStrictStorageVariant(t *testing.T) {
	empty, err := NewEmptyProjectionContent("application/octet-stream")
	if err != nil || empty.Storage != ProjectionContentEmpty || empty.ByteCount != 0 ||
		empty.Blob != nil || empty.Manifest != nil {
		t.Fatalf("empty projection content=%+v err=%v", empty, err)
	}
	forgedEmpty := empty
	forgedEmpty.ByteCount = 1
	if forgedEmpty.Validate() == nil {
		t.Fatal("empty projection content accepted a nonzero logical byte count")
	}
	if _, err := NewEmptyProjectionContent("application/json"); err == nil {
		t.Fatal("empty projection content accepted an invalid empty JSON body")
	}
	direct, bindings, blobs, err := NewPublicProjectionContent(
		"direct", "application/json", []byte(`{"direct":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if direct.Storage != ProjectionContentDirect || direct.Blob == nil ||
		direct.Manifest != nil || len(bindings) != 0 || len(blobs) != 1 {
		t.Fatalf("direct projection content=%+v bindings=%d blobs=%d", direct, len(bindings), len(blobs))
	}
	forged := direct
	manifest := direct.Blob
	forged.Manifest = manifest
	if forged.Validate() == nil {
		t.Fatal("projection content accepted simultaneous direct and chunked storage")
	}
}

func TestPrivateProjectionContentRejectsPublicPrivateRoleSwap(t *testing.T) {
	privateEmpty, err := NewPrivateEmptyProjectionContent("application/octet-stream")
	if err != nil || privateEmpty.Role != ChunkedBlobPrivateWorld {
		t.Fatalf("private empty=%+v err=%v", privateEmpty, err)
	}
	forgedPublicEmpty := privateEmpty
	forgedPublicEmpty.Role = ChunkedBlobPublicAgentKnowledge
	if privateEmpty.ValidateForRole(ChunkedBlobPublicAgentKnowledge) == nil ||
		forgedPublicEmpty.ValidateForRole(ChunkedBlobPrivateWorld) == nil {
		t.Fatal("private/public empty authority role swap was accepted")
	}
	publicEmpty, err := NewEmptyProjectionContent("application/octet-stream")
	if err != nil || publicEmpty.Role != ChunkedBlobPublicAgentKnowledge ||
		publicEmpty == privateEmpty || forgedPublicEmpty != publicEmpty {
		t.Fatal("public/private empty authorities did not bind distinct roles")
	}
	raw := append([]byte("private:"), bytes.Repeat([]byte{'x'}, MaxDirectBlobBytes)...)
	private, bindings, blobs, err := NewPrivateProjectionContent(
		"private-authority", "application/octet-stream", raw,
	)
	if err != nil || private.Role != ChunkedBlobPrivateWorld ||
		private.Storage != ProjectionContentChunked {
		t.Fatalf("private content=%+v err=%v", private, err)
	}
	byDigest := blobsByDigest(blobs)
	if err := validateProjectionContentBinding(private, bindings, byDigest); err != nil {
		t.Fatal(err)
	}
	forgedPublic := private
	forgedPublic.Role = ChunkedBlobPublicAgentKnowledge
	if err := validateProjectionContentBinding(forgedPublic, bindings, byDigest); err == nil {
		t.Fatal("private chunk manifest was accepted as public-agent knowledge")
	}
	public, publicBindings, publicBlobs, err := NewPublicProjectionContent(
		"public-authority", "application/octet-stream", raw,
	)
	if err != nil {
		t.Fatal(err)
	}
	forgedPrivate := public
	forgedPrivate.Role = ChunkedBlobPrivateWorld
	if err := validateProjectionContentBinding(
		forgedPrivate, publicBindings, blobsByDigest(publicBlobs),
	); err == nil {
		t.Fatal("public chunk manifest was accepted as private-world evidence")
	}
}

func semanticProjectionContentFixture(t *testing.T) SemanticProjectionInput {
	t.Helper()
	base := validBaseInput(t)
	base.Sources[1].Kind = "obligation_graph"
	for index := range base.Events {
		base.Events[index].MappingSchema = SemanticMappingSchemaV1
		base.Events[index].Sources = []SourceRef{base.Sources[index].Ref()}
	}
	public, publicBindings, publicBlobs, err := NewPublicProjectionContent(
		"public", "application/json", []byte(`{"public":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	episode, episodeBindings, episodeBlobs, err := NewPublicProjectionContent(
		"episode", "application/json", projectionLargeJSON(),
	)
	if err != nil {
		t.Fatal(err)
	}
	trace, traceBindings, traceBlobs, err := NewPublicProjectionContent(
		"trace", "application/json", []byte(`{"trace":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	blobs := append(base.Blobs, publicBlobs...)
	blobs = append(blobs, episodeBlobs...)
	blobs = append(blobs, traceBlobs...)
	bindings := append(publicBindings, episodeBindings...)
	bindings = append(bindings, traceBindings...)
	return SemanticProjectionInput{
		TerminalAuthority: base.TerminalAuthority,
		PublicWorldSHA256: base.PublicWorldSHA256, PublicWorldSchema: base.PublicWorldSchema,
		PublicAuthoritySHA256: base.PublicAuthoritySHA256,
		PublicBundleAuthority: public, SealedEpisodeAuthority: episode,
		ProductionTraceAuthority: trace, Sidecars: []ProjectionSidecarAuthority{},
		Sources: base.Sources, Events: base.Events, Checkpoints: base.Checkpoints,
		ChunkedBlobs: bindings, Blobs: blobs,
	}
}

func projectionLargeJSON() []byte {
	return append(append([]byte(`{"authority":"`),
		bytes.Repeat([]byte{'a'}, MaxDirectBlobBytes)...), []byte(`"}`)...)
}
