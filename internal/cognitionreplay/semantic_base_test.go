package cognitionreplay

import "testing"

func TestGenericReplayCannotMintSeriousSemanticEvidence(t *testing.T) {
	input := validBaseInput(t)
	for index := range input.Events {
		input.Events[index].MappingSchema = SemanticMappingSchemaV1
	}
	if _, err := ExportStructuralBase(input); err == nil {
		t.Fatal("structural exporter accepted caller-authored semantic mappings")
	}
	verified, err := VerifyBase(mustStructuralArtifact(t).Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Manifest().SemanticStatus != SemanticStructural ||
		verified.Manifest().ProjectionAuthority != nil {
		t.Fatal("generic structural replay acquired semantic qualification authority")
	}
}

func mustStructuralArtifact(t *testing.T) Artifact {
	t.Helper()
	artifact, err := ExportStructuralBase(validBaseInput(t))
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}
