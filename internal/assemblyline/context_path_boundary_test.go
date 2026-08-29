package assemblyline

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestContextRelevanceProjectionRedactsFilesystemIdentitiesAndHidesProvenance(t *testing.T) {
	t.Parallel()
	authority := ContextRelevanceInput{
		ExactInstruction:   "Compare /srv/private/turn.json with secret_owner.go.",
		KnownArtifactPaths: []string{"internal/private/secret_owner.go"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(
				t, "conversation", "CTX_1",
				`The prior response cited docs/private/guide.md and C:\private\notes.txt.`,
			),
		},
		MaxSelections: 1,
	}
	job, err := NewContextRelevanceSelectionJob(ContextRelevanceSelectionInput{
		Authority: authority, AcceptedCandidateIDs: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{
		"/srv/private/turn.json", "secret_owner.go", "internal/private/secret_owner.go",
		"docs/private/guide.md", `C:\private\notes.txt`,
	} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("context relevance prompt exposed filesystem identity %q: %s", hidden, prompt)
		}
	}
	if !strings.Contains(prompt, "ARTIFACT_1") {
		t.Fatalf("context relevance prompt omitted deterministic redaction: %s", prompt)
	}
	if !strings.Contains(string(job.Payload), "internal/private/secret_owner.go") {
		t.Fatalf("portable authority lost code-owned artifact provenance: %s", job.Payload)
	}
}

func TestContextRelevanceRawLeafRejectsQualifiedAndKnownArtifactIdentities(t *testing.T) {
	t.Parallel()
	authority := ContextRelevanceInput{
		ExactInstruction:   "Recall the relevant exchange.",
		KnownArtifactPaths: []string{"internal/private/secret_owner.go"},
		CandidateAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(t, "conversation", "CTX_1", "The prior exchange remains relevant."),
		},
		MaxSelections: 1,
	}
	input := ContextRelevanceSelectionInput{
		Authority: authority, AcceptedCandidateIDs: []string{},
	}
	for _, raw := range []string{
		"/private/result", "../private/result", `C:\private\result`, "secret_owner.go",
	} {
		if _, err := DecodeContextRelevanceSelectionDecision(input, raw); err == nil ||
			!strings.Contains(err.Error(), "filesystem identity") {
			t.Fatalf("context relevance leaf %q path error=%v", raw, err)
		}
	}
}

func TestContextMinificationProjectionAndRawLeafEnforceArtifactBoundary(t *testing.T) {
	t.Parallel()
	input := ContextMinificationInput{
		ExactInstruction:   "Summarize secret_owner.go without exposing /srv/private/state.",
		KnownArtifactPaths: []string{"internal/private/secret_owner.go"},
		SelectedAuthorities: []ContextCandidateAuthority{
			contextCandidateFixture(
				t, "conversation", "CTX_1",
				`The earlier exchange mentioned docs/private/guide.md and C:\private\notes.txt.`,
			),
		},
	}
	job, err := NewContextMinificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{
		"secret_owner.go", "internal/private/secret_owner.go", "/srv/private/state",
		"docs/private/guide.md", `C:\private\notes.txt`,
	} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("context minification prompt exposed filesystem identity %q: %s", hidden, prompt)
		}
	}
	for _, raw := range []string{
		"Retain /private/result.", "Retain ../private/result.", `Retain C:\private\result.`,
		"Retain secret_owner.go.",
	} {
		if _, err := DecodeContextMinificationDecision(input, raw); err == nil ||
			!strings.Contains(err.Error(), "filesystem identity") {
			t.Fatalf("context minification leaf %q path error=%v", raw, err)
		}
	}
	if _, err := DecodeContextMinificationDecision(
		input, "The earlier exchange remains relevant to the current question.",
	); err != nil {
		t.Fatalf("rejected path-free context minification leaf: %v", err)
	}
}

func TestContextSieveRequiresExplicitArtifactProvenance(t *testing.T) {
	t.Parallel()
	candidate := contextCandidateFixture(t, "conversation", "CTX_1", "The prior exchange remains relevant.")
	if _, err := NewContextRelevanceSelectionJob(ContextRelevanceSelectionInput{
		Authority: ContextRelevanceInput{
			ExactInstruction: "Recall it.", CandidateAuthorities: []ContextCandidateAuthority{candidate},
			MaxSelections: 1,
		},
		AcceptedCandidateIDs: []string{},
	}); err == nil {
		t.Fatal("context relevance accepted missing artifact provenance authority")
	}
	if _, err := NewContextMinificationJob(ContextMinificationInput{
		ExactInstruction: "Recall it.", SelectedAuthorities: []ContextCandidateAuthority{candidate},
	}); err == nil {
		t.Fatal("context minification accepted missing artifact provenance authority")
	}
}

func TestContextSieveModelProjectionsHaveNoArtifactProvenanceField(t *testing.T) {
	t.Parallel()
	for name, projection := range map[string]any{
		"relevance":    contextRelevanceSelectionProjection{},
		"minification": contextMinificationModelProjection{},
	} {
		typeOf := reflect.TypeOf(projection)
		if _, exists := typeOf.FieldByName("KnownArtifactPaths"); exists {
			t.Fatalf("%s model projection exposes current-artifact provenance", name)
		}
	}
}

func TestContextSieveSourceHasNoDirectRawTextProjection(t *testing.T) {
	t.Parallel()
	for path, forbidden := range map[string][]string{
		"context_relevance_selection.go": {
			"ExactInstruction:     input.Authority.ExactInstruction",
			"Content:     candidate.Content",
		},
		"context_minification.go": {
			"ExactInstruction: input.ExactInstruction",
			"SelectedContext[index] = authority.Content",
		},
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), "redactContextModelText(") {
			t.Fatalf("%s omits the deterministic context text redaction boundary", path)
		}
		for _, direct := range forbidden {
			if strings.Contains(string(source), direct) {
				t.Errorf("%s directly projects raw context text %q", path, direct)
			}
		}
	}
}
