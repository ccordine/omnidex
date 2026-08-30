package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateContentPresenceBuildsCodeOwnedKind(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name               string
		candidate          string
		runtimePresence    ApplicationRequirementCandidateContentPresence
		nonRuntimePresence ApplicationRequirementCandidateContentPresence
		want               string
	}{
		{
			name:               "mixed image resizer",
			candidate:          "Build software in Rust that resizes the submitted image.",
			runtimePresence:    ApplicationRequirementCandidateContentPresent,
			nonRuntimePresence: ApplicationRequirementCandidateContentPresent,
			want:               ApplicationRequirementCandidateMixed,
		},
		{
			name:               "mixed barcode scanner",
			candidate:          "Create a mobile-browser tool that scans a submitted barcode and returns its encoded value.",
			runtimePresence:    ApplicationRequirementCandidateContentPresent,
			nonRuntimePresence: ApplicationRequirementCandidateContentPresent,
			want:               ApplicationRequirementCandidateMixed,
		},
		{
			name:               "runtime image resize",
			candidate:          "Resize the submitted image.",
			runtimePresence:    ApplicationRequirementCandidateContentPresent,
			nonRuntimePresence: ApplicationRequirementCandidateContentAbsent,
			want:               ApplicationRequirementCandidateTaskLocal,
		},
		{
			name:               "runtime word sort",
			candidate:          "Sort submitted words alphabetically.",
			runtimePresence:    ApplicationRequirementCandidateContentPresent,
			nonRuntimePresence: ApplicationRequirementCandidateContentAbsent,
			want:               ApplicationRequirementCandidateTaskLocal,
		},
		{
			name:               "non-runtime application shell",
			candidate:          "Build a React application.",
			runtimePresence:    ApplicationRequirementCandidateContentAbsent,
			nonRuntimePresence: ApplicationRequirementCandidateContentPresent,
			want:               ApplicationRequirementCandidateNonRuntime,
		},
		{
			name:               "non-runtime framework",
			candidate:          "Use React.",
			runtimePresence:    ApplicationRequirementCandidateContentAbsent,
			nonRuntimePresence: ApplicationRequirementCandidateContentPresent,
			want:               ApplicationRequirementCandidateNonRuntime,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			runtimeInput := ApplicationRequirementCandidateContentPresenceInput{
				Candidate: fixture.candidate,
				Dimension: ApplicationRequirementCandidateRuntimeContentDimension,
			}
			nonRuntimeInput := ApplicationRequirementCandidateContentPresenceInput{
				Candidate: fixture.candidate,
				Dimension: ApplicationRequirementCandidateNonRuntimeContentDimension,
			}
			runtimeJob, runtimePrompt := applicationRequirementCandidateContentPresencePromptFixture(
				t,
				runtimeInput,
			)
			nonRuntimeJob, nonRuntimePrompt := applicationRequirementCandidateContentPresencePromptFixture(
				t,
				nonRuntimeInput,
			)
			if runtimeJob.Kind != WorkApplicationRequirementCandidateKind ||
				nonRuntimeJob.Kind != WorkApplicationRequirementCandidateKind ||
				runtimeJob.ID == nonRuntimeJob.ID {
				t.Fatalf(
					"presence jobs do not share one kind with dimension-bound identities: runtime=%+v non_runtime=%+v",
					runtimeJob,
					nonRuntimeJob,
				)
			}
			if !strings.Contains(runtimePrompt, "subjectless imperative") ||
				!strings.Contains(runtimePrompt, "not direct behavior for this question") ||
				strings.Contains(runtimePrompt, "behavior-defining product name") ||
				strings.Contains(runtimePrompt, "grammatical subject") {
				t.Fatalf("runtime-content prompt is not dimension-specific:\n%s", runtimePrompt)
			}
			if !strings.Contains(nonRuntimePrompt, "A subject that merely refers") ||
				!strings.Contains(nonRuntimePrompt, "when the same candidate also names runtime behavior") ||
				!strings.Contains(nonRuntimePrompt, "do not classify the candidate as a whole") ||
				strings.Contains(nonRuntimePrompt, "subjectless imperative") {
				t.Fatalf("non-runtime-content prompt is not dimension-specific:\n%s", nonRuntimePrompt)
			}

			runtimeReceipt := applicationRequirementCandidateContentPresenceFixture(
				t,
				runtimeInput,
				fixture.runtimePresence,
			)
			nonRuntimeReceipt := applicationRequirementCandidateContentPresenceFixture(
				t,
				nonRuntimeInput,
				fixture.nonRuntimePresence,
			)
			kind, resolved, err := ResolveApplicationRequirementCandidateKind(
				fixture.candidate,
				runtimeReceipt,
				nonRuntimeReceipt,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !resolved || kind.Relation != fixture.want ||
				kind.CandidateSHA256 != ExactObjectiveContextSHA(fixture.candidate) {
				t.Fatalf("resolved=%t kind=%+v want=%q", resolved, kind, fixture.want)
			}
			if err := kind.ValidateFor(ApplicationRequirementCandidateKindInput{
				Candidate: fixture.candidate,
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApplicationRequirementCandidateContentPresenceRejectsUnboundState(t *testing.T) {
	t.Parallel()
	const candidate = "Export reports as CSV."
	invalidDimension := ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate,
		Dimension: ApplicationRequirementCandidateContentDimension("other_content"),
	}
	if _, err := NewApplicationRequirementCandidateContentPresenceJob(invalidDimension); err == nil {
		t.Fatal("unregistered content dimension produced a model job")
	}
	runtimeInput := ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate,
		Dimension: ApplicationRequirementCandidateRuntimeContentDimension,
	}
	runtimeReceipt := applicationRequirementCandidateContentPresenceFixture(
		t,
		runtimeInput,
		ApplicationRequirementCandidateContentPresent,
	)
	changedCandidate := runtimeInput
	changedCandidate.Candidate = "Export records as CSV."
	if err := runtimeReceipt.ValidateFor(changedCandidate); err == nil {
		t.Fatal("runtime-content receipt validated for another candidate")
	}
	changedDimension := runtimeInput
	changedDimension.Dimension = ApplicationRequirementCandidateNonRuntimeContentDimension
	if err := runtimeReceipt.ValidateFor(changedDimension); err == nil {
		t.Fatal("runtime-content receipt validated for another dimension")
	}
	changedReceipt := runtimeReceipt
	changedReceipt.Schema = "invalid"
	if err := changedReceipt.ValidateFor(runtimeInput); err == nil {
		t.Fatal("presence receipt with an invalid schema validated")
	}
	changedReceipt = runtimeReceipt
	changedReceipt.Presence = ApplicationRequirementCandidateContentPresence("UNKNOWN")
	if err := changedReceipt.ValidateFor(runtimeInput); err == nil {
		t.Fatal("presence receipt with an unregistered value validated")
	}
	if _, err := DecodeApplicationRequirementCandidateContentPresenceResult(
		runtimeInput,
		ApplicationRequirementCandidateMixed,
	); err == nil {
		t.Fatal("retired three-way model value decoded as a presence")
	}
}

func TestApplicationRequirementCandidateKindLeavesTwoAbsentDimensionsUnresolved(t *testing.T) {
	t.Parallel()
	const candidate = "A bounded candidate with no classified content."
	runtimeReceipt := applicationRequirementCandidateContentPresenceFixture(
		t,
		ApplicationRequirementCandidateContentPresenceInput{
			Candidate: candidate,
			Dimension: ApplicationRequirementCandidateRuntimeContentDimension,
		},
		ApplicationRequirementCandidateContentAbsent,
	)
	nonRuntimeReceipt := applicationRequirementCandidateContentPresenceFixture(
		t,
		ApplicationRequirementCandidateContentPresenceInput{
			Candidate: candidate,
			Dimension: ApplicationRequirementCandidateNonRuntimeContentDimension,
		},
		ApplicationRequirementCandidateContentAbsent,
	)
	result, resolved, err := ResolveApplicationRequirementCandidateKind(
		candidate,
		runtimeReceipt,
		nonRuntimeReceipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved || result != (ApplicationRequirementCandidateKindResult{}) {
		t.Fatalf("both-absent result resolved=%t kind=%+v", resolved, result)
	}
}

func TestApplicationRequirementCandidateKindReceiptRejectsUnboundState(t *testing.T) {
	t.Parallel()
	input := ApplicationRequirementCandidateKindInput{Candidate: "Export reports as CSV."}
	result := applicationRequirementCandidateKindFixture(
		t,
		input.Candidate,
		ApplicationRequirementCandidateTaskLocal,
	)
	mutations := map[string]func(*ApplicationRequirementCandidateKindResult){
		"schema": func(value *ApplicationRequirementCandidateKindResult) {
			value.Schema = "invalid"
		},
		"candidate hash": func(value *ApplicationRequirementCandidateKindResult) {
			value.CandidateSHA256 = strings.Repeat("0", 64)
		},
		"relation": func(value *ApplicationRequirementCandidateKindResult) {
			value.Relation = "UNKNOWN"
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			changed := result
			mutate(&changed)
			if err := changed.ValidateFor(input); err == nil {
				t.Fatal("unbound candidate-kind receipt validated")
			}
		})
	}
}

func applicationRequirementCandidateContentPresencePromptFixture(
	t testing.TB,
	input ApplicationRequirementCandidateContentPresenceInput,
) (PortableJob, string) {
	t.Helper()
	job, err := NewApplicationRequirementCandidateContentPresenceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Validate(); err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, input.Candidate) != 1 {
		t.Fatalf("presence prompt lost its one exact candidate:\n%s", prompt)
	}
	for _, forbidden := range []string{
		ApplicationRequirementCandidateTaskLocal,
		ApplicationRequirementCandidateNonRuntime,
		ApplicationRequirementCandidateMixed,
		"USER REQUEST:",
		"ACCEPTED REQUIREMENT",
		"PRODUCT CONTEXT:",
		"workflow",
		"task queue",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("presence prompt exposed retired or unrelated context %q:\n%s", forbidden, prompt)
		}
	}
	if maximum, err := PortableResponseMaximumBytesForJob(job); err != nil {
		t.Fatal(err)
	} else if maximum != len(ApplicationRequirementCandidateContentPresent) {
		t.Fatalf("presence response maximum=%d", maximum)
	}
	if framing, err := PortableResponseFramingForWorkKind(job.Kind); err != nil {
		t.Fatal(err)
	} else if framing != PortableResponseFramingSingleLine {
		t.Fatalf("presence response framing=%q", framing)
	}
	return job, prompt
}

func applicationRequirementCandidateContentPresenceFixture(
	t testing.TB,
	input ApplicationRequirementCandidateContentPresenceInput,
	presence ApplicationRequirementCandidateContentPresence,
) ApplicationRequirementCandidateContentPresenceResult {
	t.Helper()
	result, err := DecodeApplicationRequirementCandidateContentPresenceResult(
		input,
		string(presence),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	return result
}

func applicationRequirementCandidateKindFixture(
	t testing.TB,
	candidate string,
	relation string,
) ApplicationRequirementCandidateKindResult {
	t.Helper()
	runtimePresence := ApplicationRequirementCandidateContentAbsent
	nonRuntimePresence := ApplicationRequirementCandidateContentAbsent
	switch relation {
	case ApplicationRequirementCandidateTaskLocal:
		runtimePresence = ApplicationRequirementCandidateContentPresent
	case ApplicationRequirementCandidateNonRuntime:
		nonRuntimePresence = ApplicationRequirementCandidateContentPresent
	case ApplicationRequirementCandidateMixed:
		runtimePresence = ApplicationRequirementCandidateContentPresent
		nonRuntimePresence = ApplicationRequirementCandidateContentPresent
	default:
		t.Fatalf("unregistered candidate kind fixture relation %q", relation)
	}
	runtimeReceipt := applicationRequirementCandidateContentPresenceFixture(
		t,
		ApplicationRequirementCandidateContentPresenceInput{
			Candidate: candidate,
			Dimension: ApplicationRequirementCandidateRuntimeContentDimension,
		},
		runtimePresence,
	)
	nonRuntimeReceipt := applicationRequirementCandidateContentPresenceFixture(
		t,
		ApplicationRequirementCandidateContentPresenceInput{
			Candidate: candidate,
			Dimension: ApplicationRequirementCandidateNonRuntimeContentDimension,
		},
		nonRuntimePresence,
	)
	result, resolved, err := ResolveApplicationRequirementCandidateKind(
		candidate,
		runtimeReceipt,
		nonRuntimeReceipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved {
		t.Fatal("candidate kind fixture did not resolve")
	}
	return result
}
