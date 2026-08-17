package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestMissingCriterionRepairUsesOnlyCodeOwnedCriterionIdentity(t *testing.T) {
	t.Parallel()

	const missingCriterion = "Selecting an appointment visibly marks it selected."
	program := directCodingGroundingFixtureProgram(
		t, "appointment schedule", "select appointments",
		[]string{"The appointment schedule is visible.", missingCriterion},
		`expect(screen.getByText("Appointments")).toBeInTheDocument();`,
	)
	corrected := directCodingAcceptanceSourceWithBody(program,
		`expect(screen.getByText("Appointments")).toBeInTheDocument();
expect(screen.getByText("Selected appointment")).toBeInTheDocument();`,
	)
	reviews := 0
	correctionPrompt := ""
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			switch job.Kind {
			case assemblyline.WorkApplicationAcceptanceGroundingReview:
				reviews++
				prompt, _, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if strings.Contains(prompt, program.Generated["acceptance.001"]) ||
					strings.Contains(prompt, "VerifyFeature001") || strings.Contains(prompt, "src/") {
					return assemblyline.PortableResult{}, fmt.Errorf("grounding review leaked source or path authority")
				}
				var input assemblyline.ApplicationAcceptanceGroundingReviewInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if reviews == 1 {
					return directCodingPortableCandidate(job, directCodingGroundingMatrixJSON(
						t, input, func(_ string, criterionID string) bool {
							return criterionID != "criterion_002"
						},
					)), nil
				}
				return directCodingPortableCandidate(job, directCodingAcceptedGroundingJSON(t, input)), nil
			case assemblyline.WorkFragmentCorrection:
				prompt, _, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				correctionPrompt = prompt
				return directCodingPortableCandidate(job, corrected), nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
		},
	}
	if err := ensureDirectCodingAcceptanceGrounding(runtime, "reviewer", "corrector", &program); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"EXPECTED_FROZEN_CRITERION criterion_002", missingCriterion,
	} {
		if !strings.Contains(correctionPrompt, required) {
			t.Fatalf("criterion correction omitted %q:\n%s", required, correctionPrompt)
		}
	}
	for _, forbidden := range []string{"unsupported_site_id", "reviewer said", "src/features/"} {
		if strings.Contains(correctionPrompt, forbidden) {
			t.Fatalf("criterion correction gained prose/path authority %q:\n%s", forbidden, correctionPrompt)
		}
	}
}

func TestAcceptanceGroundingCorrectionHasNoFixedAttemptCount(t *testing.T) {
	t.Parallel()

	program := directCodingGroundingFixtureProgram(
		t, "status board", "show records", []string{
			"The record collection is visible.", "The current record state is visible.",
		},
		groundingProgressBody("Records 0"),
	)
	reviews, corrections := 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			switch job.Kind {
			case assemblyline.WorkApplicationAcceptanceGroundingReview:
				reviews++
				var input assemblyline.ApplicationAcceptanceGroundingReviewInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if reviews <= 4 {
					siteID := directCodingGroundingSiteIDForOperation(t, input, "testing_library_query:getByText")
					return directCodingPortableCandidate(job, directCodingGroundingMatrixJSON(
						t, input, func(actualSiteID string, _ string) bool {
							return actualSiteID != siteID
						},
					)), nil
				}
				return directCodingPortableCandidate(job, directCodingAcceptedGroundingJSON(t, input)), nil
			case assemblyline.WorkFragmentCorrection:
				corrections++
				return directCodingPortableCandidate(
					job, directCodingAcceptanceSourceWithBody(program, groundingProgressBody(fmt.Sprintf("Records %d", corrections))),
				), nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
		},
	}
	if err := ensureDirectCodingAcceptanceGrounding(runtime, "reviewer", "corrector", &program); err != nil {
		t.Fatal(err)
	}
	if reviews != 5 || corrections != 4 {
		t.Fatalf("reviews=%d corrections=%d want 5/4", reviews, corrections)
	}
}

func TestAcceptanceGroundingStopsRepeatedSourceCycle(t *testing.T) {
	t.Parallel()

	program := directCodingGroundingFixtureProgram(
		t, "status board", "show records", []string{"The record collection is visible."},
		groundingProgressBody("Records A"),
	)
	input := directCodingGroundingInput(t, program, "acceptance.001")
	program.AcceptanceGroundingSeen = map[string]map[string]struct{}{
		"acceptance.001": {input.SourceSHA256: {}},
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{}, fmt.Errorf("repeated source reached model work %s", job.Kind)
		},
	}
	err := ensureDirectCodingAcceptanceGrounding(runtime, "reviewer", "corrector", &program)
	if err == nil || !strings.Contains(err.Error(), "repeated a prior source state") {
		t.Fatalf("cycle error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("repeated source invoked %d model calls", calls)
	}
}

func groundingProgressBody(label string) string {
	return `expect(screen.getByText("` + label + `")).toBeInTheDocument();`
}

func directCodingAcceptanceSourceWithBody(program directCodingProgram, body string) string {
	block, _ := directCodingTypeScriptBlueprintBlock(program.TypeScript, "acceptance.001")
	return block.Signature + " { " + body + " }"
}
