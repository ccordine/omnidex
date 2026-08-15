package assemblyline

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestApplicationJobSpecificationReviewIsBoundToAuthorityAndRetainedState(t *testing.T) {
	t.Parallel()
	authority := applicationJobSpecificationTestInput(1)
	retained := applicationJobSpecificationTestValue()
	input, err := NewApplicationJobSpecificationReviewInput(authority, retained, 1)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildApplicationJobSpecificationReviewPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{
		string(authority.Surface), authority.ProductQuote, authority.FocusedRequirement.SourceQuote,
		retained.Objective, retained.RequiredBehaviors[0], retained.AcceptanceCriteria[0],
		`"user_authority"`, `"derived_candidate"`, "observable acceptance criteria collectively cover",
		"one concise diagnostic finding", "finding_evidence",
	} {
		if !strings.Contains(prompt, exact) {
			t.Fatalf("review prompt omitted %q:\n%s", exact, prompt)
		}
	}
	for _, forbidden := range []string{"file_path", "tool_catalog", "depends_on", "completion"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("review prompt leaked %q", forbidden)
		}
	}
	job, err := NewApplicationJobSpecificationReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationJobSpecificationReview {
		t.Fatalf("review work kind=%q", job.Kind)
	}
}

func TestApplicationJobSpecificationReviewWireAcceptsOrNamesOneDerivedField(t *testing.T) {
	t.Parallel()
	input, err := NewApplicationJobSpecificationReviewInput(
		applicationJobSpecificationTestInput(1), applicationJobSpecificationTestValue(), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := ApplicationJobSpecificationReviewResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	branches, ok := schema["oneOf"].([]any)
	if schema["type"] != "object" || !ok || len(branches) != 2 {
		t.Fatalf("review schema is not a closed accept-or-repair object: %#v", schema)
	}
	accept := branches[0].(map[string]any)
	repairBranch := branches[1].(map[string]any)
	if !reflect.DeepEqual(accept["required"], []string{"decision"}) ||
		!reflect.DeepEqual(repairBranch["required"], []string{
			"decision", "field", "finding", "finding_evidence",
		}) {
		t.Fatalf("review schema branches do not make legal responses complete: %#v", branches)
	}
	for _, branch := range []map[string]any{accept, repairBranch} {
		if branch["type"] != "object" || branch["additionalProperties"] != false {
			t.Fatalf("review schema branch is not a complete closed object: %#v", branch)
		}
	}
	accepted, err := DecodeApplicationJobSpecificationReview(input, `{"decision":"accept"}`)
	if err != nil || accepted.Decision != ApplicationJobSpecificationReviewAccept {
		t.Fatalf("accepted review=%+v error=%v", accepted, err)
	}
	repair, err := DecodeApplicationJobSpecificationReview(input, `{"decision":"repair","field":"required_behaviors","finding":"The behaviors do not state the focused user action and observable result.","finding_evidence":"Users can add and remove mixer channels."}`)
	if err != nil {
		t.Fatal(err)
	}
	if repair.Decision != ApplicationJobSpecificationReviewRepair ||
		repair.Field != ApplicationJobSpecificationRequiredBehaviorsField ||
		repair.Finding != "The behaviors do not state the focused user action and observable result." ||
		repair.FindingEvidence != "Users can add and remove mixer channels." {
		t.Fatalf("repair review=%+v", repair)
	}

	invalid := []string{
		`{"decision":"accept","field":"objective"}`,
		`{"decision":"accept","finding":"Unwanted."}`,
		`{"decision":"accept","finding_evidence":"Unwanted."}`,
		`{"decision":"repair"}`,
		`{"decision":"repair","field":"objective"}`,
		`{"decision":"repair","field":"objective","finding":"The objective is not local."}`,
		`{"decision":"repair","field":"path","finding":"The objective is not local.","finding_evidence":"Implement"}`,
		`{"decision":"repair","field":"objective","finding":"","finding_evidence":"Implement"}`,
		`{"decision":"repair","field":"objective","finding":"The objective is not local.","finding_evidence":""}`,
		`{"decision":"repair","field":"objective","finding":"The objective is not local.","finding_evidence":"Implement","replacement":"Write a replacement."}`,
		`{"decision":"repair","field":"objective","finding":"The objective is not local.","finding_evidence":"Implement","path":"src/app.tsx"}`,
	}
	for _, raw := range invalid {
		if _, err := DecodeApplicationJobSpecificationReview(input, raw); err == nil {
			t.Fatalf("accepted invalid review %s", raw)
		}
	}
	if _, err := NewApplicationJobSpecificationReviewInput(
		applicationJobSpecificationTestInput(1), applicationJobSpecificationTestValue(), 41,
	); err != nil {
		t.Fatalf("progressing review attempt was rejected by a numeric ceiling: %v", err)
	}
}

func TestApplicationJobSpecificationReviewResultDecodesFromItsPortableAuthority(t *testing.T) {
	t.Parallel()
	input, err := NewApplicationJobSpecificationReviewInput(
		applicationJobSpecificationTestInput(1), applicationJobSpecificationTestValue(), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := NewApplicationJobSpecificationReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	for raw, want := range map[string]ApplicationJobSpecificationReviewDecision{
		`{"decision":"accept"}`: ApplicationJobSpecificationReviewAccept,
		`{"decision":"repair","field":"acceptance_criteria","finding":"The checks do not cover the required behavior.","finding_evidence":"Adding a channel displays"}`: ApplicationJobSpecificationReviewRepair,
	} {
		review, err := DecodeApplicationJobSpecificationReviewResult(job, raw)
		if err != nil {
			t.Fatal(err)
		}
		if review.Decision != want {
			t.Fatalf("review=%+v want decision=%s", review, want)
		}
	}
	if _, err := DecodeApplicationJobSpecificationReviewResult(
		job, `{"decision":"accept","field":"objective"}`,
	); err == nil {
		t.Fatal("portable review result bypassed its bound semantic decoder")
	}
}

func TestApplicationJobSpecificationResultDecodesFromItsPortableAuthority(t *testing.T) {
	t.Parallel()
	authority := applicationJobSpecificationTestInput(1)
	job, err := NewApplicationJobSpecificationJob(authority)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{
		"objective":"Implement independently controllable mixer channels.",
		"required_behaviors":["Users can change one mixer channel independently."],
		"acceptance_criteria":["Changing one channel leaves another channel unchanged."]
	}`
	specification, err := DecodeApplicationJobSpecificationResult(job, raw)
	if err != nil {
		t.Fatal(err)
	}
	if specification.Objective != "Implement independently controllable mixer channels." {
		t.Fatalf("specification=%+v", specification)
	}
	if _, err := DecodeApplicationJobSpecificationResult(job, `{
		"objective":"",
		"required_behaviors":["Users can change one mixer channel independently."],
		"acceptance_criteria":["Changing one channel leaves another channel unchanged."]
	}`); err == nil {
		t.Fatal("portable specification result bypassed production semantic validation")
	}
}

func TestApplicationJobSpecificationRepairReplacesExactlyOneTopLevelField(t *testing.T) {
	t.Parallel()
	authority := applicationJobSpecificationTestInput(1)
	retained := applicationJobSpecificationTestValue()
	reviewInput, err := NewApplicationJobSpecificationReviewInput(authority, retained, 1)
	if err != nil {
		t.Fatal(err)
	}
	review, err := DecodeApplicationJobSpecificationReview(reviewInput, `{"decision":"repair","field":"required_behaviors","finding":"The behaviors do not state how users independently control a channel.","finding_evidence":"Users can add and remove mixer channels."}`)
	if err != nil {
		t.Fatal(err)
	}
	input, err := NewApplicationJobSpecificationRepairInput(authority, retained, review, 1)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := renderApplicationJobSpecificationRepair(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{
		retained.RequiredBehaviors[0], review.Finding, review.FindingEvidence,
		`"user_authority"`, `"current_derived_value"`, `"target_derived_field"`,
		`"review_finding"`, `"finding_evidence"`,
		"Observable does not mean numeric",
	} {
		if !strings.Contains(prompt, exact) {
			t.Fatalf("repair prompt omitted %q:\n%s", exact, prompt)
		}
	}
	for _, forbidden := range []string{
		authority.ProductQuote,
		authority.AcceptedRequirements[2].SourceQuote,
		retained.Objective,
		retained.AcceptanceCriteria[0],
		"file_path",
		"tool_catalog",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("repair prompt exposed reviewer-authored authority %q:\n%s", forbidden, prompt)
		}
	}
	properties := schema["properties"].(map[string]any)
	if !reflect.DeepEqual(sortedJobSpecificationKeys(properties), []string{"required_behaviors"}) {
		t.Fatalf("repair schema retargeted fields: %#v", properties)
	}
	definition := properties["required_behaviors"].(map[string]any)
	if definition["type"] != "array" || definition["minItems"] != 1 || definition["maxItems"] != 4 {
		t.Fatalf("repair schema does not replace one entire bounded behavior field: %#v", definition)
	}
	patch, err := DecodeApplicationJobSpecificationRepair(input, `{"required_behaviors":["Users can add and remove independently controllable mixer channels."]}`)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := ApplyApplicationJobSpecificationRepair(input, retained, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.Objective, retained.Objective) ||
		!reflect.DeepEqual(updated.AcceptanceCriteria, retained.AcceptanceCriteria) {
		t.Fatalf("repair changed retained fields: before=%+v after=%+v", retained, updated)
	}
	if reflect.DeepEqual(updated.RequiredBehaviors, retained.RequiredBehaviors) {
		t.Fatal("repair did not replace its named field")
	}
	payload, err := newApplicationJobSpecificationRepairPortablePayload(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.ObservedValueSHA256) != 64 ||
		payload.ObservedValueSHA256 != review.observedValueSHA256 {
		t.Fatalf("portable repair omitted code-owned current-field binding: %+v", payload)
	}
	tampered := payload
	tampered.ObservedValueSHA256 = strings.Repeat("0", 64)
	if err := tampered.validate(); err == nil {
		t.Fatal("portable repair accepted a current-field hash that did not match retained state")
	}
}

func TestApplicationJobSpecificationRepairSupportsOnlyThreeSemanticFields(t *testing.T) {
	t.Parallel()
	authority := applicationJobSpecificationTestInput(1)
	retained := applicationJobSpecificationTestValue()
	tests := map[ApplicationJobSpecificationField]string{
		ApplicationJobSpecificationObjectiveField:          `{"objective":"Implement independently controllable mixer channels."}`,
		ApplicationJobSpecificationRequiredBehaviorsField:  `{"required_behaviors":["Expose independent controls for each mixer channel."]}`,
		ApplicationJobSpecificationAcceptanceCriteriaField: `{"acceptance_criteria":["Changing one channel leaves another channel unchanged."]}`,
	}
	for field, raw := range tests {
		field, raw := field, raw
		t.Run(string(field), func(t *testing.T) {
			t.Parallel()
			review := applicationJobSpecificationRepairReview(t, authority, retained, field)
			input, err := NewApplicationJobSpecificationRepairInput(authority, retained, review, 2)
			if err != nil {
				t.Fatal(err)
			}
			patch, err := DecodeApplicationJobSpecificationRepair(input, raw)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyApplicationJobSpecificationRepair(input, retained, patch); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApplicationJobSpecificationRepairRejectsNoOpRetargetAndAuthorityDrift(t *testing.T) {
	t.Parallel()
	authority := applicationJobSpecificationTestInput(1)
	retained := applicationJobSpecificationTestValue()
	review := applicationJobSpecificationRepairReview(
		t, authority, retained, ApplicationJobSpecificationObjectiveField,
	)
	input, err := NewApplicationJobSpecificationRepairInput(authority, retained, review, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"objective":"Implement interactive mixer channels for the browser music studio."}`,
		`{"acceptance_criteria":["Changed."]}`,
		`{"objective":"Changed.","required_behaviors":["Also changed."]}`,
	} {
		if _, err := DecodeApplicationJobSpecificationRepair(input, raw); err == nil {
			t.Fatalf("accepted invalid repair %s", raw)
		}
	}
	patch, err := DecodeApplicationJobSpecificationRepair(input, `{"objective":"Implement independently controllable mixer channels."}`)
	if err != nil {
		t.Fatal(err)
	}
	drifted := retained
	drifted.AcceptanceCriteria = []string{"Different retained state."}
	if _, err := ApplyApplicationJobSpecificationRepair(input, drifted, patch); err == nil {
		t.Fatal("applied repair to drifted retained state")
	}
	if _, err := NewApplicationJobSpecificationRepairInput(authority, retained, review, 41); err != nil {
		t.Fatalf("productive repair identity was given an arbitrary numeric ceiling: %v", err)
	}
	if _, err := NewApplicationJobSpecificationRepairInput(authority, retained, review, 0); err == nil {
		t.Fatal("accepted a non-positive repair identity")
	}
	accepted := review
	accepted.Decision = ApplicationJobSpecificationReviewAccept
	accepted.Field = ""
	accepted.Finding = ""
	accepted.FindingEvidence = ""
	if _, err := NewApplicationJobSpecificationRepairInput(authority, retained, accepted, 1); err == nil {
		t.Fatal("constructed repair from accepted review")
	}
}

func applicationJobSpecificationRepairReview(
	t testing.TB,
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) ApplicationJobSpecificationReview {
	t.Helper()
	input, err := NewApplicationJobSpecificationReviewInput(authority, retained, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]string{
		"decision":         string(ApplicationJobSpecificationReviewRepair),
		"field":            string(field),
		"finding":          "The named field does not state the focused user action and observable result.",
		"finding_evidence": applicationJobSpecificationReviewEvidenceForField(retained, field),
	})
	if err != nil {
		t.Fatal(err)
	}
	review, err := DecodeApplicationJobSpecificationReview(input, string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return review
}

func TestApplicationJobSpecificationReviewRejectsEvidenceAbsentFromExactCurrentField(t *testing.T) {
	t.Parallel()
	authority := applicationJobSpecificationTestInput(1)
	retained := applicationJobSpecificationTestValue()
	input, err := NewApplicationJobSpecificationReviewInput(authority, retained, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeApplicationJobSpecificationReview(input, `{"decision":"repair","field":"objective","finding":"The objective includes an unrelated export capability.","finding_evidence":"export capability"}`)
	var evidenceErr *ApplicationJobSpecificationReviewEvidenceError
	if !errors.As(err, &evidenceErr) {
		t.Fatalf("ungrounded reviewer evidence error=%T %v", err, err)
	}
	if evidenceErr.Field != ApplicationJobSpecificationObjectiveField ||
		evidenceErr.FindingEvidence != "export capability" ||
		len(evidenceErr.ObservedValueSHA256) != 64 ||
		len(evidenceErr.RetainedAuthoritySHA256) != 64 {
		t.Fatalf("evidence error=%+v", evidenceErr)
	}

	retry, err := NewApplicationJobSpecificationReviewRetryInput(
		authority, retained, 2, *evidenceErr,
	)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildApplicationJobSpecificationReviewPrompt(retry)
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{
		`"prior_validation_failure"`, `"finding_evidence":"export capability"`,
		retained.Objective, "does not occur in the exact current named field",
	} {
		if !strings.Contains(prompt, exact) {
			t.Fatalf("review retry prompt omitted %q:\n%s", exact, prompt)
		}
	}

	drifted := retained
	drifted.AcceptanceCriteria = []string{"A different retained criterion."}
	if _, err := NewApplicationJobSpecificationReviewRetryInput(
		authority, drifted, 3, *evidenceErr,
	); err == nil {
		t.Fatal("review retry accepted a validation failure bound to different field state")
	}
}

func applicationJobSpecificationReviewEvidenceForField(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) string {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		return retained.Objective
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return retained.RequiredBehaviors[0]
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return retained.AcceptanceCriteria[0]
	default:
		return ""
	}
}
