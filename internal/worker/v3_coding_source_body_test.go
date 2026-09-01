package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLanguageFragmentCorrectionSendsOnlyProvenSpanAndSplicesInSameJob(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language:  "javascript",
		Dialect:   "ECMAScript 2022",
		Signature: "function Sum(left, right)",
		Behavior:  "Return the sum of left and right.",
	}
	initialBody := "const total = left - right;\nreturn total;"
	wantedQuestion := "Fix this expression so it adds left and right."
	var originalJobID, correctedJobID, correctedModel, correctionInput string
	rejected, accepted, released := 0, 0, 0
	runtime := typedWorkerRuntime{
		Context:     context.Background(),
		MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			originalJobID = job.ID
			if model != "fixture-model" {
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected model %q", model)
			}
			return exactSourceBodyTestResult(t, job, initialBody), nil
		},
		Correct: func(
			job assemblyline.PortableJob,
			model string,
			correction assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			correctedJobID, correctedModel = job.ID, model
			var err error
			correctionInput, err = correction.ModelInput()
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			if correction.Mutable() != "left - right" {
				return assemblyline.PortableResult{}, fmt.Errorf("mutable span=%q", correction.Mutable())
			}
			return exactSourceBodyTestResult(t, job, "left + right"), nil
		},
		Release: func(assemblyline.PortableJob) error { released++; return nil },
		Finalize: func(
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			validationErr error,
		) error {
			if validationErr != nil {
				rejected++
			} else {
				accepted++
			}
			return nil
		},
	}
	validate := func(
		input assemblyline.FragmentGenerationInput,
		body string,
	) (string, error) {
		if start := strings.Index(body, "left - right"); start >= 0 {
			defect, err := assemblyline.NewSourceBodyDefect(
				body, start, start+len("left - right"), wantedQuestion,
				errors.New("addition expression uses subtraction"),
			)
			if err != nil {
				return "", err
			}
			return "", defect
		}
		return validateDirectCodingJavaScriptFragment(input, body)
	}

	source, err := runDirectCodingLanguageFragmentWorker(
		runtime, "fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "source.001", Input: input, Validate: validate,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if originalJobID == "" || correctedJobID != originalJobID || correctedModel != "fixture-model" {
		t.Fatalf(
			"correction identity=(job %q model %q), want original job %q and model",
			correctedJobID, correctedModel, originalJobID,
		)
	}
	if correctionInput != wantedQuestion+"\n\nleft - right" {
		t.Fatalf("correction input=%q", correctionInput)
	}
	for _, forbidden := range []string{
		input.Signature, "const total =", "return total", initialBody,
		"same job", "preserve", "implementation body", "JSON",
	} {
		if strings.Contains(correctionInput, forbidden) {
			t.Fatalf("correction input exposed non-mutable state %q: %q", forbidden, correctionInput)
		}
	}
	wantedBody := "const total = left + right;\nreturn total;"
	if strings.Count(source, input.Signature) != 1 || !strings.Contains(source, wantedBody) {
		t.Fatalf("code-owned declaration=%q", source)
	}
	if rejected != 1 || accepted != 1 || released != 0 {
		t.Fatalf("outcomes rejected=%d accepted=%d released=%d", rejected, accepted, released)
	}
}

func TestLanguageFragmentAdvancesSoleSpliceBeforeOpaqueCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)", Behavior: "Return the sum.",
	}
	choice := func(description, value string) assemblyline.OpaqueModelChoice {
		t.Helper()
		result, err := assemblyline.NewOpaqueModelChoice(description, value)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	left := choice("JavaScript function parameter named \"left\"", "left")
	right := choice("JavaScript function parameter named \"right\"", "right")
	initial := "return badOne + badTwo;"
	advanced := "return left + badTwo;"
	advanceCalls, correctionCalls := 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(t, job, initial), nil
		},
		Correct: func(
			job assemblyline.PortableJob,
			_ string,
			correction assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			correctionCalls++
			if correction.Mutable() != "badTwo" || correction.BaseCandidate() != advanced {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"opaque correction base=%q mutable=%q",
					correction.BaseCandidate(), correction.Mutable(),
				)
			}
			return exactSourceBodyTestResult(t, job, "B"), nil
		},
		AdvanceSource: func(
			_ assemblyline.PortableJob,
			_ string,
			expectedBase string,
			updatedBase string,
		) error {
			advanceCalls++
			if expectedBase != initial || updatedBase != advanced {
				return fmt.Errorf("advance %q -> %q", expectedBase, updatedBase)
			}
			return nil
		},
		Release: func(assemblyline.PortableJob) error { return nil },
		Finalize: func(
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			_ error,
		) error {
			return nil
		},
	}
	validate := func(
		input assemblyline.FragmentGenerationInput,
		body string,
	) (string, error) {
		if start := strings.Index(body, "badOne"); start >= 0 {
			defect, err := assemblyline.NewSourceBodyIdentifierDefect(
				body, start, start+len("badOne"), "Which value belongs here?",
				errors.New("first unresolved identifier"),
				[]assemblyline.OpaqueModelChoice{left},
			)
			if err != nil {
				return "", err
			}
			return "", defect
		}
		if start := strings.Index(body, "badTwo"); start >= 0 {
			defect, err := assemblyline.NewSourceBodyIdentifierDefect(
				body, start, start+len("badTwo"), "Which value belongs here?",
				errors.New("second unresolved identifier"),
				[]assemblyline.OpaqueModelChoice{left, right},
			)
			if err != nil {
				return "", err
			}
			return "", defect
		}
		return validateDirectCodingJavaScriptFragment(input, body)
	}

	result, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "source.sole-then-opaque", Input: input, Validate: validate,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if advanceCalls != 1 || correctionCalls != 1 {
		t.Fatalf("advance calls=%d correction calls=%d", advanceCalls, correctionCalls)
	}
	want := "function Sum(left, right) {\nreturn left + right;\n}"
	if result != want {
		t.Fatalf("result=%q; want %q", result, want)
	}
}

func TestLanguageFragmentSpanCorrectionExhaustionReleasesOnlyItsJob(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Value()", Behavior: "Return one value.",
	}
	corrections, rejections, releases := 0, 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(t, job, "return wrong;"), nil
		},
		Correct: func(
			job assemblyline.PortableJob,
			_ string,
			correction assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			corrections++
			if correction.Mutable() != "wrong" {
				return assemblyline.PortableResult{}, fmt.Errorf("mutable=%q", correction.Mutable())
			}
			return exactSourceBodyTestResult(t, job, "wrong"), nil
		},
		Release: func(assemblyline.PortableJob) error { releases++; return nil },
		Finalize: func(
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			validationErr error,
		) error {
			if validationErr == nil {
				return fmt.Errorf("invalid fixture unexpectedly passed")
			}
			rejections++
			return nil
		},
	}
	validate := func(
		_ assemblyline.FragmentGenerationInput,
		body string,
	) (string, error) {
		start := strings.Index(body, "wrong")
		if start < 0 {
			return "", fmt.Errorf("fixture lost exact defect")
		}
		defect, err := assemblyline.NewSourceBodyDefect(
			body, start, start+len("wrong"),
			"Fix this identifier so it names the required value.",
			errors.New("identifier does not name the required value"),
		)
		if err != nil {
			return "", err
		}
		return "", defect
	}

	_, err := runDirectCodingLanguageFragmentWorker(
		runtime, "fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "source.002", Input: input, Validate: validate,
		},
	)
	if err == nil {
		t.Fatal("unchanged defective span unexpectedly passed")
	}
	if corrections != 1 || rejections != 2 || releases != 1 {
		t.Fatalf("corrections=%d rejections=%d releases=%d", corrections, rejections, releases)
	}
}

func TestLanguageFragmentExtractsCompleteDeclarationWithoutCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Value()", Behavior: "Return one value.",
	}
	corrections, releases := 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return exactSourceBodyTestResult(
				t, job, "function Value() { return 1; }",
			), nil
		},
		Correct: func(
			assemblyline.PortableJob,
			string,
			assemblyline.SourceBodyCorrection,
		) (assemblyline.PortableResult, error) {
			corrections++
			return assemblyline.PortableResult{}, fmt.Errorf("whole-body correction was invoked")
		},
		Release: func(assemblyline.PortableJob) error { releases++; return nil },
		Finalize: func(
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			_ error,
		) error {
			return nil
		},
	}

	source, err := runDirectCodingLanguageFragmentWorker(
		runtime, "fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "source.003", Input: input,
			Validate: validateDirectCodingJavaScriptFragment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if source != "function Value() {\nreturn 1;\n}" {
		t.Fatalf("extracted source=%q", source)
	}
	if corrections != 0 || releases != 0 {
		t.Fatalf("corrections=%d releases=%d", corrections, releases)
	}
}

func exactSourceBodyTestResult(
	t *testing.T,
	job assemblyline.PortableJob,
	response string,
) assemblyline.PortableResult {
	t.Helper()
	projection, err := assemblyline.NewExactPortableResultProjection(response)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.PortableResult{
		JobID: job.ID, Candidate: response, Projection: &projection,
	}
}
