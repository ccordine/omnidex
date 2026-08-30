package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestApplicationRequirementNonRuntimeExclusionRebindsRemainingAuthority(t *testing.T) {
	t.Parallel()
	request := "Build a browser counter that shows a count in one source file."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	authority := ApplicationRequirementCoverageInput{
		UserRequest: request, Context: context,
		AcceptedRequirements: []string{}, ExcludedCandidates: []string{}, ZeroDeltas: []ApplicationRequirementCandidateZeroDelta{},
	}
	coverage, err := DecodeApplicationRequirementCoverageLeaf(
		authority, ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationRequirementCandidateInput{Authority: authority, Coverage: coverage}
	const candidate = "Keep the project in one source file."
	kindInput := ApplicationRequirementCandidateKindInput{Candidate: candidate}
	kind, err := DecodeApplicationRequirementCandidateKindResult(
		kindInput, ApplicationRequirementCandidateNonRuntime,
	)
	if err != nil {
		t.Fatal(err)
	}
	rebound, err := RebindApplicationRequirementAfterNonRuntimeExclusion(
		input, candidate, kind,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rebound.Authority.AcceptedRequirements, []string{}) ||
		!reflect.DeepEqual(rebound.Authority.ExcludedCandidates, []string{candidate}) ||
		rebound.Coverage.Relation != ApplicationRequirementRemains {
		t.Fatalf("rebound=%+v", rebound)
	}
	if err := rebound.Coverage.ValidateFor(rebound.Authority); err != nil {
		t.Fatal(err)
	}
	if rebound.Coverage.AuthoritySHA256 == input.Coverage.AuthoritySHA256 {
		t.Fatal("exclusion rebind reused the stale authority hash")
	}
	if len(input.Authority.ExcludedCandidates) != 0 {
		t.Fatal("exclusion rebind mutated prior authority")
	}
}

func TestApplicationRequirementNonRuntimeExclusionRebindRejectsInvalidAuthority(t *testing.T) {
	t.Parallel()
	valid := applicationRequirementCandidateFixture(t, applicationRequirementZeroDeltaAuthority(t))
	const candidate = "Add a generic test obligation."
	kindInput := ApplicationRequirementCandidateKindInput{Candidate: candidate}
	nonRuntime, err := DecodeApplicationRequirementCandidateKindResult(
		kindInput, ApplicationRequirementCandidateNonRuntime,
	)
	if err != nil {
		t.Fatal(err)
	}
	taskLocal, err := DecodeApplicationRequirementCandidateKindResult(
		kindInput, ApplicationRequirementCandidateTaskLocal,
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		input     ApplicationRequirementCandidateInput
		candidate string
		kind      ApplicationRequirementCandidateKindResult
		want      string
	}{
		"stale receipt": {
			input: func() ApplicationRequirementCandidateInput {
				changed := valid
				changed.Coverage.AuthoritySHA256 = strings.Repeat("0", 64)
				return changed
			}(),
			candidate: candidate, kind: nonRuntime, want: "authority hash",
		},
		"task local": {
			input: valid, candidate: candidate, kind: taskLocal,
			want: "NON_RUNTIME_CONSTRAINT",
		},
		"accepted duplicate": {
			input: valid, candidate: valid.Authority.AcceptedRequirements[0],
			kind: func() ApplicationRequirementCandidateKindResult {
				input := ApplicationRequirementCandidateKindInput{
					Candidate: valid.Authority.AcceptedRequirements[0],
				}
				result, decodeErr := DecodeApplicationRequirementCandidateKindResult(
					input, ApplicationRequirementCandidateNonRuntime,
				)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				return result
			}(),
			want: "accepted requirement",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, rebindErr := RebindApplicationRequirementAfterNonRuntimeExclusion(
				test.input, test.candidate, test.kind,
			)
			if rebindErr == nil || !strings.Contains(rebindErr.Error(), test.want) {
				t.Fatalf("rebind error=%v want=%q", rebindErr, test.want)
			}
		})
	}
}
