package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestPlainTextFragmentCandidateRejectsEveryFilesystemIdentityForm(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		[]string{"handbooks/field-guide.txt"},
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		candidate string
	}{
		{
			name:      "POSIX absolute release note",
			candidate: "The deployment record is stored at /srv/releases/deployment-note.txt.\n",
		},
		{
			name:      "traversal training reference",
			candidate: "The workshop continues with ../archive/training-reference.txt.\n",
		},
		{
			name:      "Windows qualified support handoff",
			candidate: "The support handoff is in C:\\ProgramData\\Support\\handoff.txt.\n",
		},
		{
			name:      "known bare field guide",
			candidate: "New volunteers receive field-guide.txt during orientation.\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
				Execute: testPortableExecutor(func(_ string, _ string, _ string) (string, error) {
					return test.candidate, nil
				}),
			}
			_, err := runDirectCodingLanguageFragmentWorker(
				runtime, "fragment-model", directCodingLanguageGenerationJob{
					Subject: "opaque-text-node",
					Input: assemblyline.FragmentGenerationInput{
						Language:  assemblyline.TextFragmentLanguage,
						Dialect:   assemblyline.TextFragmentDialect,
						Signature: assemblyline.TextFragmentSignature,
						Behavior:  "Return one concise informational paragraph.",
					},
					Project: assemblyline.ProjectTextFragment,
					Validate: func(_ assemblyline.FragmentGenerationInput, candidate string) (string, error) {
						return candidate, assemblyline.ValidateTextFragment(candidate)
					},
				},
			)
			if err == nil || !strings.Contains(
				err.Error(), "language fragment candidate field 1 contains filesystem identity",
			) {
				t.Fatalf("plain-text filesystem identity error=%v", err)
			}
		})
	}
}

func TestPlainTextFragmentCandidateKeepsAdapterBareTargetRejection(t *testing.T) {
	t.Parallel()
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string) (string, error) {
			return "A customer receipt is recorded in receipt.txt.\n", nil
		}),
	}
	_, err := runDirectCodingLanguageFragmentWorker(
		runtime, "fragment-model", directCodingLanguageGenerationJob{
			Subject: "opaque-text-node",
			Input: assemblyline.FragmentGenerationInput{
				Language:  assemblyline.TextFragmentLanguage,
				Dialect:   assemblyline.TextFragmentDialect,
				Signature: assemblyline.TextFragmentSignature,
				Behavior:  "Return one concise customer-facing sentence.",
			},
			Project: assemblyline.ProjectTextFragment,
			Validate: func(_ assemblyline.FragmentGenerationInput, candidate string) (string, error) {
				if err := validatePlainTextPathBlindValue(candidate); err != nil {
					return "", err
				}
				return candidate, assemblyline.ValidateTextFragment(candidate)
			},
		},
	)
	if err == nil || !strings.Contains(
		err.Error(), "path-blind plain-text source context contains artifact identity",
	) {
		t.Fatalf("adapter-recognized bare target error=%v", err)
	}
}
