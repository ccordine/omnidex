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
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			prompts = append(prompts, prompt)
			kinds = append(kinds, job.Kind)
			if len(prompts) == 1 {
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: "delete_file",
				}, nil
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: string(assemblyline.KnownArtifactMustBeAbsent),
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
		"CURRENT_REJECTED_LEAF:", "delete_file",
		"EXACT_GROUNDED_DEFECT:", "unsupported",
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

func TestKnownArtifactTruthPartitionPreservesOneDecisionPerQuote(t *testing.T) {
	t.Parallel()
	quotes := []string{
		"One new release note artifact must exist.",
		"The obsolete semantic artifact must no longer exist.",
		"The retained behavior must be updated.",
	}
	answers := []assemblyline.KnownArtifactTruth{
		assemblyline.OnePlainTextArtifactMustExist,
		assemblyline.KnownArtifactMustBeAbsent,
		assemblyline.KnownArtifactTruthNotApplicable,
	}
	call := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			if job.Kind != assemblyline.WorkKnownArtifactTruth || call >= len(answers) {
				t.Fatalf("unexpected semantic call %d kind=%q", call, job.Kind)
			}
			answer := answers[call]
			call++
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: string(answer),
			}, nil
		},
	}
	partition, err := classifyKnownArtifactTruthQuotes(
		runtime, "semantic", quotes, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if call != len(quotes) || len(partition.MustExist) != 1 ||
		partition.MustExist[0] != quotes[0] || len(partition.MustBeAbsent) != 1 ||
		partition.MustBeAbsent[0] != quotes[1] || len(partition.NotApplicable) != 1 ||
		partition.NotApplicable[0] != quotes[2] {
		t.Fatalf("partition=%+v calls=%d", partition, call)
	}
}
