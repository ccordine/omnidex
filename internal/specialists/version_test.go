package specialists

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSkillVersionRequiresValidatedActiveState(t *testing.T) {
	t.Parallel()

	version := validSkillVersion(t)
	version.Status = SkillStatusActive
	version.Validation = nil

	err := version.Validate()
	if err == nil || !strings.Contains(err.Error(), "validation evidence") {
		t.Fatalf("Validate() error=%v, want validation evidence failure", err)
	}
}

func TestSkillContentHashCoversEveryExecutableContractField(t *testing.T) {
	t.Parallel()

	version := validSkillVersion(t)
	first, err := version.Spec.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	version.Spec.Instructions = "Perform a different bounded operation."
	second, err := version.Spec.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("skill content hash ignored instructions")
	}
}

func TestSkillContentHashCanonicalizesSchemaFormatting(t *testing.T) {
	t.Parallel()

	first := validSkillVersion(t).Spec
	second := first
	second.inputSchemaRaw = json.RawMessage("{\n  \"additionalProperties\": false,\n  \"type\": \"object\"\n}")
	firstHash, err := SkillContentHash(first, SkillKindCodeProcedure)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := SkillContentHash(second, SkillKindCodeProcedure)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("equivalent JSON schemas produced different hashes: %s != %s", firstHash, secondHash)
	}
}

func validSkillVersion(t *testing.T) SkillVersion {
	t.Helper()
	spec := Spec{
		ID: "learned_0123456789abcdef0123456789abcdef", Purpose: "Perform one bounded test operation.",
		Instructions:    "Return the tested value only.",
		inputSchemaRaw:  json.RawMessage(`{"type":"object","additionalProperties":false}`),
		outputSchemaRaw: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
	hash, err := SkillContentHash(spec, SkillKindCodeProcedure)
	if err != nil {
		t.Fatal(err)
	}
	return SkillVersion{
		Spec: spec, Version: 1, Status: SkillStatusActive,
		Source: SkillSourceLearned, Kind: SkillKindCodeProcedure, CreatedByJobID: int64Pointer(1), ContentSHA256: hash,
	}
}

func int64Pointer(value int64) *int64 { return &value }
