package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestKnownArtifactTruthCorrectionChangesOnlyTruthLeaf(t *testing.T) {
	t.Parallel()
	input := assemblyline.KnownArtifactTruthInput{
		RequirementQuote: "The known semantic artifact declaring Obsolete must no longer exist.",
	}
	var prompts []string
	var kinds []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			prompt, schema, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			prompts = append(prompts, prompt)
			kinds = append(kinds, job.Kind)
			if len(prompts) == 1 {
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: `{"schema":"omnidex.known-artifact-truth.v1","truth":"delete_file"}`,
				}, nil
			}
			properties := schema["properties"].(map[string]any)
			if len(properties) != 1 || properties["truth"] == nil {
				t.Fatalf("correction schema may alter more than truth: %#v", schema)
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: `{"truth":"known_artifact_must_be_absent"}`,
			}, nil
		},
	}
	decision, err := classifyKnownArtifactTruth(runtime, "semantic", input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Truth != assemblyline.KnownArtifactMustBeAbsent {
		t.Fatalf("decision=%+v", decision)
	}
	if len(prompts) != 2 || kinds[0] != assemblyline.WorkKnownArtifactTruth ||
		kinds[1] != assemblyline.WorkResponseCorrection {
		t.Fatalf("calls=%v prompts=%d", kinds, len(prompts))
	}
	for _, required := range []string{
		"ORIGINAL_SEMANTIC_QUESTION:", input.RequirementQuote,
		"CURRENT_INVALID_RESPONSE:", `"truth":"delete_file"`,
		"EXACT_VALIDATION_DEFECT:", "unsupported",
	} {
		if !strings.Contains(prompts[1], required) {
			t.Fatalf("correction omitted retained one-leaf authority %q: %s", required, prompts[1])
		}
	}
}

func TestDirectArtifactAbsenceTruthDisagreementFailsLoudly(t *testing.T) {
	t.Parallel()
	quote := "ARTIFACT_1 must no longer exist."
	err := validateDirectArtifactAbsenceTruth(
		[]string{quote},
		[]assemblyline.ArtifactDirective{{
			Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactForbid,
		}},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("truth disagreement error=%v", err)
	}
}
