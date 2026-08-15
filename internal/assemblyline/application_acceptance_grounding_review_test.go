package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func acceptanceGroundingFixture(t *testing.T) (ApplicationAcceptanceGroundingReviewInput, string) {
	t.Helper()
	source := `function VerifyNotifications(): void {
  expect(screen.getByRole("checkbox", { name: "Email notices" })).toBeVisible();
  expect(screen.getByRole("checkbox", { name: "Email notices" })).toBeChecked();
}`
	input, err := NewApplicationAcceptanceGroundingReviewInput(ApplicationTaskContext{
		WorkloadSHA256: strings.Repeat("a", 64),
		Task: ApplicationTaskContextTask{
			TaskID: "task_004",
			AcceptanceCriteria: []string{
				"A user can find the email-notice control by its accessible name.",
				"Activating the control visibly changes its checked state.",
			},
		},
	}, source, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	return input, source
}

func TestApplicationAcceptanceGroundingReviewAcceptsOnlyCompleteBoundMappings(t *testing.T) {
	t.Parallel()

	input, source := acceptanceGroundingFixture(t)
	job, err := NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewResponseCorrectionJob(job, "one grounding leaf is invalid"); err == nil {
		t.Fatal("grounding response received unscoped correction authority")
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		source, input.TaskID, input.WorkloadSHA256, input.SourceSHA256, "privateControl",
		`"locators"`, `"start_byte"`, `"start_line"`, `"source_sha256"`, `"tsx"`,
		"workspace", "path", "tool",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("review prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	for _, required := range []string{
		"criterion_001", "criterion_002", "site_001", "site_002", "site_003", "site_004",
		"site_001__criterion_001", "site_004__criterion_002",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("review prompt omitted %q:\n%s", required, prompt)
		}
	}
	if schema["additionalProperties"] != false || schema["oneOf"] != nil {
		t.Fatalf("review schema is not one closed fixed object: %#v", schema)
	}

	raw := acceptanceGroundingLeafFixtureRaw(t, input, map[string]bool{
		"site_001__criterion_001": true,
		"site_002__criterion_001": true,
		"site_003__criterion_002": true,
		"site_004__criterion_002": true,
	})
	review, err := DecodeApplicationAcceptanceGroundingReview(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := AcceptApplicationAcceptanceGroundingReview(input, review)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.ValidateFor(input, source); err != nil {
		t.Fatal(err)
	}
	authorized, err := receipt.AuthorizesFeatureFailureAt(input, source, true, 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !authorized {
		t.Fatal("criterion-grounded query failure was not authorized for feature routing")
	}
	changed := strings.Replace(source, "toBeChecked", "toBeDisabled", 1)
	if err := receipt.ValidateFor(input, changed); err == nil {
		t.Fatal("receipt remained valid for changed source")
	}
}

func TestApplicationAcceptanceGroundingReviewDoesNotTreatResponseBytesAsCorrectness(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	raw := strings.Repeat(" ", maxPortableCandidateBytes+1) +
		acceptanceGroundingLeafFixtureRaw(t, input, map[string]bool{
			"site_001__criterion_001": true,
			"site_002__criterion_001": true,
			"site_003__criterion_002": true,
			"site_004__criterion_002": true,
		})
	if _, err := DecodeApplicationAcceptanceGroundingReview(input, raw); err != nil {
		t.Fatalf("semantically valid grounding review was rejected only for byte length: %v", err)
	}
}

func TestApplicationAcceptanceGroundingReviewRejectsInventedOrIncompleteAuthority(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	validMappings := `{"site_id":"site_001","authority_ids":["criterion_001"]},` +
		`{"site_id":"site_002","authority_ids":["criterion_001"]},` +
		`{"site_id":"site_003","authority_ids":["criterion_002"]},` +
		`{"site_id":"site_004","authority_ids":["criterion_002"]}`
	tests := map[string]string{
		"unknown authority":          `{"decision":"accept","mappings":[{"site_id":"site_001","authority_ids":["criterion_999"]},{"site_id":"site_002","authority_ids":["criterion_001"]},{"site_id":"site_003","authority_ids":["criterion_002"]},{"site_id":"site_004","authority_ids":["criterion_002"]}]}`,
		"missing site":               `{"decision":"accept","mappings":[{"site_id":"site_001","authority_ids":["criterion_001"]},{"site_id":"site_002","authority_ids":["criterion_001"]},{"site_id":"site_003","authority_ids":["criterion_002"]}]}`,
		"duplicate site":             `{"decision":"accept","mappings":[{"site_id":"site_001","authority_ids":["criterion_001"]},{"site_id":"site_001","authority_ids":["criterion_001"]},{"site_id":"site_003","authority_ids":["criterion_002"]},{"site_id":"site_004","authority_ids":["criterion_002"]}]}`,
		"missing criterion":          `{"decision":"accept","mappings":[{"site_id":"site_001","authority_ids":["criterion_001"]},{"site_id":"site_002","authority_ids":["criterion_001"]},{"site_id":"site_003","authority_ids":["criterion_001"]},{"site_id":"site_004","authority_ids":["criterion_001"]}]}`,
		"platform covers product":    `{"decision":"accept","mappings":[{"site_id":"site_001","authority_ids":["platform_wait_harness"]},{"site_id":"site_002","authority_ids":["criterion_001"]},{"site_id":"site_003","authority_ids":["criterion_002"]},{"site_id":"site_004","authority_ids":["criterion_002"]}]}`,
		"out of order":               `{"decision":"accept","mappings":[{"site_id":"site_002","authority_ids":["criterion_001"]},{"site_id":"site_001","authority_ids":["criterion_001"]},{"site_id":"site_003","authority_ids":["criterion_002"]},{"site_id":"site_004","authority_ids":["criterion_002"]}]}`,
		"accept extra repair":        `{"decision":"accept","mappings":[` + validMappings + `],"unsupported_site_id":"site_001"}`,
		"repair prose":               `{"decision":"repair","unsupported_site_id":"site_001","reason":"invented count"}`,
		"repair both":                `{"decision":"repair","unsupported_site_id":"site_001","missing_criterion_id":"criterion_002"}`,
		"repair null mappings":       `{"decision":"repair","unsupported_site_id":"site_001","mappings":null}`,
		"accept null repair field":   `{"decision":"accept","mappings":[` + validMappings + `],"unsupported_site_id":null}`,
		"unknown repair site":        `{"decision":"repair","unsupported_site_id":"site_999"}`,
		"platform missing criterion": `{"decision":"repair","missing_criterion_id":"platform_wait_harness"}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationAcceptanceGroundingReview(input, raw); err == nil {
				t.Fatal("invalid grounding review was accepted")
			}
		})
	}
}

func TestApplicationAcceptanceGroundingPrebindsWaitHarnessOutsideModelReview(t *testing.T) {
	t.Parallel()

	source := `async function VerifyConnection(): Promise<void> {
  await waitFor(() => expect(screen.getByText("Connected")).toBeVisible());
}`
	input, err := NewApplicationAcceptanceGroundingReviewInput(ApplicationTaskContext{
		WorkloadSHA256: strings.Repeat("e", 64),
		Task: ApplicationTaskContextTask{TaskID: "task_008", AcceptanceCriteria: []string{
			"The connected state is visibly reported.",
		}},
	}, source, false, []AcceptanceGroundingAuthority{{
		ID: "platform_wait_harness", Kind: AcceptanceGroundingPlatformInvariant,
		Statement: "The registered harness may wait for observable behavior.", Operations: []string{"harness_call:waitFor"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildApplicationAcceptanceGroundingReviewPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"site_001", "harness_call:waitFor", "platform_wait_harness"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("wait harness entered model review: %s", prompt)
		}
	}
	review, err := DecodeApplicationAcceptanceGroundingReview(
		input,
		acceptanceGroundingLeafFixtureRaw(t, input, map[string]bool{
			"site_002__criterion_001": true,
			"site_003__criterion_001": true,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := AcceptApplicationAcceptanceGroundingReview(input, review)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Mappings) != 3 ||
		!reflect.DeepEqual(receipt.Mappings[0].AuthorityIDs, []string{"platform_wait_harness"}) {
		t.Fatalf("receipt omitted prebound wait harness: %+v", receipt.Mappings)
	}
}
