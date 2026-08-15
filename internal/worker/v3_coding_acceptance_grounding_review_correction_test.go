package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestAcceptanceGroundingReviewCorrectsMoreThanThreeRetainedLeaves(t *testing.T) {
	t.Parallel()

	program := directCodingGroundingFixtureProgram(
		t, "status browser", "show service status",
		[]string{"The service status is visibly reported."},
		`expect(screen.getByText("Ready")).toBeVisible();
expect(screen.getByText("Healthy")).toBeVisible();
expect(screen.getByText("Connected")).toBeVisible();`,
	)
	input := directCodingGroundingInput(t, program, "acceptance.001")
	fields := directCodingGroundingMatrixFields(input)
	if len(fields) <= 3 {
		t.Fatalf("fixture exposes %d matrix leaves; need more than three", len(fields))
	}
	original, err := assemblyline.NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}

	var jobs []assemblyline.PortableJob
	var models []string
	var finalized []error
	correction := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			jobs = append(jobs, job)
			models = append(models, model)
			switch job.Kind {
			case assemblyline.WorkApplicationAcceptanceGroundingReview:
				return directCodingPortableCandidate(job, `{}`), nil
			case assemblyline.WorkResponseCorrection:
				var payload assemblyline.ResponseCorrectionInput
				if err := json.Unmarshal(job.Payload, &payload); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if payload.Original.ID != original.ID || payload.Original.Kind != original.Kind ||
					string(payload.Original.Payload) != string(original.Payload) {
					return assemblyline.PortableResult{}, fmt.Errorf("correction changed immutable grounding authority")
				}
				if correction >= len(fields) || payload.TargetField != fields[correction] {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"correction target=%s want %s", payload.TargetField, fields[correction],
					)
				}
				if !strings.Contains(payload.ValidationFailure, "RETAINED_STATE_SHA256=") {
					return assemblyline.PortableResult{}, fmt.Errorf("correction lacks retained-state identity")
				}
				prompt, _, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if strings.Contains(prompt, directCodingAcceptedGroundingJSON(t, input)) {
					return assemblyline.PortableResult{}, fmt.Errorf("correction prompt reconstructed retained mappings")
				}
				for _, acceptedField := range fields[:correction] {
					if strings.Contains(prompt, fmt.Sprintf(`%q:true`, acceptedField)) {
						return assemblyline.PortableResult{}, fmt.Errorf(
							"correction prompt exposed retained leaf %s", acceptedField,
						)
					}
				}
				patch := directCodingGroundingRaw(t, map[string]bool{fields[correction]: true})
				correction++
				return directCodingPortableCandidate(job, patch), nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
		},
		Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
			finalized = append(finalized, validationErr)
			return nil
		},
	}
	review, err := runDirectCodingAcceptanceGroundingReview(runtime, "reviewer", "acceptance.001", input)
	if err != nil {
		t.Fatal(err)
	}
	if review.Decision != assemblyline.AcceptanceGroundingAccept {
		t.Fatalf("review=%+v", review)
	}
	if correction != len(fields) || correction <= 3 || len(jobs) != correction+1 {
		t.Fatalf("corrections=%d fields=%d jobs=%d", correction, len(fields), len(jobs))
	}
	for index, job := range jobs {
		want := assemblyline.WorkResponseCorrection
		if index == 0 {
			want = assemblyline.WorkApplicationAcceptanceGroundingReview
		}
		if job.Kind != want || models[index] != "reviewer" {
			t.Fatalf("call %d kind/model=%s/%s want %s/reviewer", index, job.Kind, models[index], want)
		}
	}
	if len(finalized) != len(jobs) || finalized[len(finalized)-1] != nil {
		t.Fatalf("finalized=%v", finalized)
	}
	for index, validationErr := range finalized[:len(finalized)-1] {
		if validationErr == nil {
			t.Fatalf("rejected result %d was finalized as accepted", index)
		}
	}
}

func TestAcceptanceGroundingCorrectionIdentityIsByteBoundedAndOmitsCriterionProse(t *testing.T) {
	t.Parallel()

	criterion := strings.Repeat("界", 512)
	input, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
		assemblyline.ApplicationTaskContext{
			WorkloadSHA256: strings.Repeat("a", 64),
			Task: assemblyline.ApplicationTaskContextTask{
				TaskID: "task_001", AcceptanceCriteria: []string{criterion},
			},
		},
		`function VerifyFeature001(): void {
  expect(screen.getByText("Ready")).toBeVisible();
}`,
		false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	fields := directCodingGroundingMatrixFields(input)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(),
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			if job.Kind == assemblyline.WorkApplicationAcceptanceGroundingReview {
				return directCodingPortableCandidate(job, `{}`), nil
			}
			var payload assemblyline.ResponseCorrectionInput
			if err := json.Unmarshal(job.Payload, &payload); err != nil {
				return assemblyline.PortableResult{}, err
			}
			if len(payload.ValidationFailure) > 1200 || strings.Contains(payload.ValidationFailure, criterion) ||
				!strings.Contains(payload.ValidationFailure, payload.TargetField) ||
				!strings.Contains(payload.ValidationFailure, "INVALID_KIND=absent") {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"unsafe correction identity bytes=%d value=%q",
					len(payload.ValidationFailure), payload.ValidationFailure,
				)
			}
			return directCodingPortableCandidate(
				job, directCodingGroundingRaw(t, map[string]bool{payload.TargetField: true}),
			), nil
		},
		Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
			if validationErr != nil && (len(validationErr.Error()) > 1200 ||
				strings.Contains(validationErr.Error(), criterion)) {
				return fmt.Errorf("unbounded persisted rejection: %d bytes", len(validationErr.Error()))
			}
			return nil
		},
	}
	review, err := runDirectCodingAcceptanceGroundingReview(
		runtime, "reviewer", "acceptance.001", input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if review.Decision != assemblyline.AcceptanceGroundingAccept || calls != len(fields)+1 {
		t.Fatalf("review=%+v calls=%d fields=%d", review, calls, len(fields))
	}
}
