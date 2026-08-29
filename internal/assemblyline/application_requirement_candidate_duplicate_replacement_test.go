package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateDuplicateReplacementBindsExactDefect(
	t *testing.T,
) {
	t.Parallel()
	input := applicationRequirementDuplicateReplacementFixture(t)
	job, err := NewApplicationRequirementCandidateDuplicateReplacementJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePortableJobForRenderer(job, PortableRendererV1); err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.GenerationAuthority.Authority.UserRequest,
		input.GenerationAuthority.Authority.AcceptedRequirements[0],
		input.GenerationAuthority.Authority.ExcludedCandidates[0],
		input.GenerationAuthority.Coverage.Relation,
		input.CurrentCandidate,
		input.Duplicate.Set,
		"zero-based index 0",
		input.Defect,
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("duplicate-replacement prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"workspace path", "file path", "worker", "orchestrator"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("duplicate-replacement prompt exposed forbidden context %q:\n%s", forbidden, prompt)
		}
	}

	const replacement = "The user can increment the current count."
	leaf, err := DecodeApplicationRequirementCandidateDuplicateReplacementLeaf(
		input, replacement,
	)
	if err != nil || leaf != replacement {
		t.Fatalf("replacement=%q error=%v", leaf, err)
	}
	if _, err := DecodeApplicationRequirementCandidateDuplicateReplacementLeaf(
		input, input.CurrentCandidate,
	); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("unchanged duplicate replacement error=%v", err)
	}

	// The station decoder owns every deterministic validity check that must
	// reject the response before its outcome can be persisted as resolved.
	input.GenerationAuthority.Authority.AcceptedRequirements = append(
		input.GenerationAuthority.Authority.AcceptedRequirements,
		"The user can reset the current count.",
	)
	input.GenerationAuthority = applicationRequirementCandidateFixture(
		t, input.GenerationAuthority.Authority,
	)
	_, err = DecodeApplicationRequirementCandidateDuplicateReplacementLeaf(
		input, input.GenerationAuthority.Authority.AcceptedRequirements[1],
	)
	if err == nil || !strings.Contains(err.Error(), "accepted requirement") {
		t.Fatalf("accepted duplicate replacement error=%v", err)
	}
	_, err = DecodeApplicationRequirementCandidateDuplicateReplacementLeaf(
		input, input.GenerationAuthority.Authority.ExcludedCandidates[0],
	)
	if err == nil || !strings.Contains(err.Error(), "excluded non-runtime") {
		t.Fatalf("excluded duplicate replacement error=%v", err)
	}

	if maximum, err := PortableResponseMaximumBytesForJob(job); err != nil {
		t.Fatal(err)
	} else if maximum != maxRequirementQuoteBytes {
		t.Fatalf("duplicate-replacement response maximum=%d", maximum)
	}
	if framing, err := PortableResponseFramingForWorkKind(job.Kind); err != nil {
		t.Fatal(err)
	} else if framing != PortableResponseFramingNaturalMultiline {
		t.Fatalf("duplicate-replacement response framing=%q", framing)
	}

}

func TestApplicationRequirementCandidateDuplicateReplacementRejectsUngroundedIdentity(
	t *testing.T,
) {
	t.Parallel()
	valid := applicationRequirementDuplicateReplacementFixture(t)
	mutations := map[string]func(*ApplicationRequirementCandidateDuplicateReplacementInput){
		"coverage receipt": func(input *ApplicationRequirementCandidateDuplicateReplacementInput) {
			input.GenerationAuthority.Coverage.AuthoritySHA256 = strings.Repeat("0", 64)
		},
		"duplicate set": func(input *ApplicationRequirementCandidateDuplicateReplacementInput) {
			input.Duplicate.Set = "UNKNOWN"
		},
		"negative index": func(input *ApplicationRequirementCandidateDuplicateReplacementInput) {
			input.Duplicate.Index = -1
		},
		"outside index": func(input *ApplicationRequirementCandidateDuplicateReplacementInput) {
			input.Duplicate.Index = 1
		},
		"different indexed bytes": func(input *ApplicationRequirementCandidateDuplicateReplacementInput) {
			input.CurrentCandidate = "The user can increment the current count."
		},
		"defect": func(input *ApplicationRequirementCandidateDuplicateReplacementInput) {
			input.Defect = "UNKNOWN"
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			changed := valid
			mutate(&changed)
			if _, err := NewApplicationRequirementCandidateDuplicateReplacementJob(changed); err == nil {
				t.Fatal("ungrounded duplicate replacement opened")
			}
		})
	}

	excluded := valid
	excluded.CurrentCandidate = excluded.GenerationAuthority.Authority.ExcludedCandidates[0]
	excluded.Duplicate = ApplicationRequirementCandidateDuplicateIdentity{
		Set: ApplicationRequirementDuplicateExcludedNonRuntimeCandidate, Index: 0,
	}
	if _, err := NewApplicationRequirementCandidateDuplicateReplacementJob(excluded); err != nil {
		t.Fatalf("exact excluded-candidate identity was rejected: %v", err)
	}
}

func applicationRequirementDuplicateReplacementFixture(
	t *testing.T,
) ApplicationRequirementCandidateDuplicateReplacementInput {
	t.Helper()
	request := "Build a browser counter that displays and increments a count in one source file."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	authority := ApplicationRequirementCoverageInput{
		UserRequest:          request,
		Context:              context,
		AcceptedRequirements: []string{"The browser shows the current count."},
		ExcludedCandidates:   []string{"Use one source file."},
	}
	return ApplicationRequirementCandidateDuplicateReplacementInput{
		GenerationAuthority: applicationRequirementCandidateFixture(t, authority),
		CurrentCandidate:    authority.AcceptedRequirements[0],
		Duplicate: ApplicationRequirementCandidateDuplicateIdentity{
			Set: ApplicationRequirementDuplicateAcceptedRequirement, Index: 0,
		},
		Defect: ApplicationRequirementDuplicateCandidateDefect,
	}
}
