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

func TestSkillVersionRejectsInvalidLifecycleTransition(t *testing.T) {
	t.Parallel()

	if err := ValidateSkillTransition(SkillStatusCandidate, SkillStatusActive); err == nil {
		t.Fatal("candidate activated without validation")
	}
	if err := ValidateSkillTransition(SkillStatusCandidate, SkillStatusValidating); err != nil {
		t.Fatalf("candidate -> validating rejected: %v", err)
	}
	if err := ValidateSkillTransition(SkillStatusValidating, SkillStatusActive); err != nil {
		t.Fatalf("validating -> active rejected: %v", err)
	}
	if err := ValidateSkillTransition(SkillStatusRejected, SkillStatusActive); err == nil {
		t.Fatal("rejected skill was resurrected")
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
	firstHash, err := SkillContentHash(first, SkillKindBootstrapSpecialist)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := SkillContentHash(second, SkillKindBootstrapSpecialist)
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
		ID: "bootstrap_test", Purpose: "Perform one bounded test operation.",
		Instructions: "Return the tested value only.", ContextBudget: 512,
		inputSchemaRaw:  json.RawMessage(`{"type":"object","additionalProperties":false}`),
		outputSchemaRaw: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
	hash, err := SkillContentHash(spec, SkillKindBootstrapSpecialist)
	if err != nil {
		t.Fatal(err)
	}
	return SkillVersion{
		Spec: spec, Version: 1, Status: SkillStatusValidating,
		Source: SkillSourceBootstrap, Kind: SkillKindBootstrapSpecialist, ContentSHA256: hash,
	}
}
