package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestDesiredArtifactCandidateUsesCompleteSameJobAuthority(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		signature string
		request   directCodingRequest
	}{
		{
			name: "project authority", signature: "func FromProject() int",
			request: directCodingRequest{
				Instruction: "Keep the current behavior.",
				AdditionalAuthority: []string{
					"Add func FromProject() int as an independent artifact.",
				},
			},
		},
		{
			name: "same-job feedback", signature: "func FromFeedback() int",
			request: directCodingRequest{
				Instruction: "Keep the current behavior.",
				Feedback: []string{
					"Add func FromFeedback() int as an independent artifact.",
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			authority := existingRepositoryAuthority(test.request)
			_, quote, missing, err := explicitMissingGoArtifactCandidate(
				authority, []string{test.signature + " as an independent artifact."},
				repositoryfacts.Analysis{},
			)
			if err != nil || !missing || !strings.Contains(quote, test.signature) {
				t.Fatalf("signature=%q missing=%t quote=%q error=%v", test.signature, missing, quote, err)
			}
		})
	}
}

func TestDesiredArtifactDeletionBindsExactAbsenceQuoteAndIgnoresPreservedArtifact(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	graph, err := compileDesiredArtifactDeletion(
		"Remove first.go while preserving sub/second.go.",
		[]string{
			"Remove ARTIFACT_1 because its owned behavior is obsolete.",
			"Preserve ARTIFACT_2 unchanged.",
		},
		[]assemblyline.ArtifactDirective{
			{Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactForbid},
			{Token: "ARTIFACT_2", Disposition: assemblyline.ArtifactProtect},
		},
		[]assemblyline.ArtifactIdentity{
			{Token: "ARTIFACT_1", Value: "first.go"},
			{Token: "ARTIFACT_2", Value: "sub/second.go"},
		},
		snapshot, analysis,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Artifacts) != 1 || graph.Artifacts[0].MustExist ||
		graph.Artifacts[0].RequirementQuote != "Remove ARTIFACT_1 because its owned behavior is obsolete." {
		t.Fatalf("graph artifacts=%+v", graph.Artifacts)
	}
}

func TestDesiredArtifactDeletionRejectsMissingOrAmbiguousAbsenceQuote(t *testing.T) {
	t.Parallel()
	for _, quotes := range [][]string{
		{"Preserve ARTIFACT_2."},
		{"Remove ARTIFACT_10."},
		{"Remove ARTIFACT_1.", "ARTIFACT_1 must be absent."},
	} {
		if _, err := exactArtifactAbsenceRequirementQuote("ARTIFACT_1", quotes); err == nil {
			t.Fatalf("invalid absence quotes accepted: %#v", quotes)
		}
	}
}

func TestDesiredArtifactGraphRejectsUnconsumedMixedTransitions(t *testing.T) {
	t.Parallel()
	directives := []assemblyline.ArtifactDirective{
		{Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactForbid},
	}
	if err := validateDesiredDeletionFeatureCoverage(
		[]string{"Remove ARTIFACT_1.", "Add func Added() int as an independent artifact."},
		directives,
	); err == nil || !strings.Contains(err.Error(), "mixed create, modify, and delete") {
		t.Fatalf("mixed delete/create coverage error=%v", err)
	}
	if err := validateDesiredCreationFeatureCoverage(
		"Add func Added() int as an independent artifact.",
		[]string{
			"Add func Added() int as an independent artifact.",
			"Change the existing return value.",
		},
		nil,
	); err == nil || !strings.Contains(err.Error(), "mixed create and modify") {
		t.Fatalf("mixed create/modify coverage error=%v", err)
	}
}
