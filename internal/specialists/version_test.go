package specialists

import (
	"reflect"
	"testing"
)

func TestSkillVersionAcceptsExactActiveRetrievalState(t *testing.T) {
	t.Parallel()

	version := validSkillVersion(t)
	if err := version.Validate(); err != nil {
		t.Fatalf("Validate() rejected active retrieval state: %v", err)
	}
}

func TestLearnedSkillTypesContainOnlyRetrievalAuthority(t *testing.T) {
	t.Parallel()

	for name, fixture := range map[string]struct {
		value any
		want  []string
	}{
		"spec": {
			value: Spec{},
			want:  []string{"ID", "Purpose", "Instructions"},
		},
		"version": {
			value: SkillVersion{},
			want: []string{
				"Spec", "Version", "Status", "Source", "Kind", "CreatedByJobID", "ContentSHA256",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			typeOf := reflect.TypeOf(fixture.value)
			got := make([]string, typeOf.NumField())
			for index := range got {
				got[index] = typeOf.Field(index).Name
			}
			if !reflect.DeepEqual(got, fixture.want) {
				t.Fatalf("fields=%v want retrieval-only fields %v", got, fixture.want)
			}
		})
	}
}

func TestSkillContentHashCoversEveryRetainedContractField(t *testing.T) {
	t.Parallel()

	base := validSkillVersion(t)
	first, err := SkillContentHash(base.Spec, base.Kind)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*SkillVersion){
		func(version *SkillVersion) { version.Spec.ID = "learned_fedcba9876543210fedcba9876543210" },
		func(version *SkillVersion) { version.Spec.Purpose = "Perform another bounded operation." },
		func(version *SkillVersion) { version.Spec.Instructions = "Return a different tested value only." },
		func(version *SkillVersion) { version.Kind = SkillKind("another_kind") },
	}
	for index, mutate := range mutations {
		candidate := base
		mutate(&candidate)
		second, err := SkillContentHash(candidate.Spec, candidate.Kind)
		if err != nil {
			t.Fatalf("mutation %d: %v", index, err)
		}
		if first == second {
			t.Fatalf("skill content hash ignored retained field mutation %d", index)
		}
	}
}

func validSkillVersion(t *testing.T) SkillVersion {
	t.Helper()
	spec := Spec{
		ID: "learned_0123456789abcdef0123456789abcdef", Purpose: "Perform one bounded test operation.",
		Instructions: "Return the tested value only.",
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
