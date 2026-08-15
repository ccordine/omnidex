package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPortableSemanticDispatchDoesNotInventSixteenKiBPromptCeiling(t *testing.T) {
	t.Parallel()

	t.Run("application intent", func(t *testing.T) {
		t.Parallel()
		request := "Build an application described by " + strings.Repeat("x", 20*1024)
		applicationContext, err := assemblyline.BootstrapApplicationContext(
			request, assemblyline.ApplicationWorkspaceEmpty, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		job, err := assemblyline.NewApplicationIntentJob(
			assemblyline.ApplicationIntentInput{
				UserRequest: request, Context: applicationContext,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		dispatched := false
		runtime := typedWorkerRuntime{
			Context: context.Background(), MaxAttempts: 1,
			Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
				prompt, _, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if len(prompt) <= 16*1024 {
					return assemblyline.PortableResult{}, fmt.Errorf("fixture prompt=%d want >16384", len(prompt))
				}
				dispatched = true
				return directCodingPortableCandidate(job, `{}`), nil
			},
		}
		if _, err := runDirectCodingSemanticCall[map[string]any](
			runtime, "semantic", "large_application_intent", job, nil,
			func(map[string]any) error { return nil },
		); err != nil {
			t.Fatal(err)
		}
		if !dispatched {
			t.Fatal("large application intent was not dispatched")
		}
	})

	t.Run("acceptance observation grounding", func(t *testing.T) {
		t.Parallel()
		var source strings.Builder
		source.WriteString("async function VerifyLargeAcceptance(): Promise<void> {\n")
		for index := 0; index < 40; index++ {
			fmt.Fprintf(&source, "expect(screen.getByText(%q)).toBeInTheDocument();\n",
				fmt.Sprintf("Visible record %03d %s", index, strings.Repeat("y", 380)))
		}
		source.WriteString("}")
		input, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
			assemblyline.ApplicationTaskContext{
				WorkloadSHA256: strings.Repeat("a", 64),
				Task: assemblyline.ApplicationTaskContextTask{
					TaskID: "task_001", AcceptanceCriteria: []string{"The visible records are shown."},
				},
			},
			source.String(), true, directCodingBrowserAcceptancePlatformAuthorities(),
		)
		if err != nil {
			t.Fatal(err)
		}
		dispatched := false
		runtime := typedWorkerRuntime{
			Context: context.Background(),
			Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
				prompt, _, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if len(prompt) <= 16*1024 {
					return assemblyline.PortableResult{}, fmt.Errorf("fixture prompt=%d want >16384", len(prompt))
				}
				dispatched = true
				return directCodingPortableCandidate(job, directCodingAcceptedGroundingJSON(t, input)), nil
			},
		}
		if _, err := runDirectCodingAcceptanceGroundingReview(runtime, "reviewer", "large", input); err != nil {
			t.Fatal(err)
		}
		if !dispatched {
			t.Fatal("large acceptance grounding review was not dispatched")
		}
	})
}

func TestObsoleteSemanticPromptByteRulerIsAbsent(t *testing.T) {
	t.Parallel()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve semantic prompt resource test path")
	}
	workerDir := filepath.Dir(testFile)
	for _, name := range []string{"v3_coding_semantic_call.go", "v3_application_workload_resolution.go"} {
		raw, err := os.ReadFile(filepath.Join(workerDir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"maxDirectCodingSemanticPromptBytes",
			"semantic correction exceeds the",
			"application job specification prompt exceeds",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains obsolete semantic byte ruler %q", name, forbidden)
			}
		}
	}
}
