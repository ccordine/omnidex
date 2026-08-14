package assemblyline

import (
	"encoding/json"
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
		"rather than merely repeating a noun", "every required behavior is covered",
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

func TestApplicationJobSpecificationReviewWireAcceptsOrNamesOneExactDefect(t *testing.T) {
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
		!reflect.DeepEqual(repairBranch["required"], []string{"decision", "field", "defect"}) {
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
	repair, err := DecodeApplicationJobSpecificationReview(input, `{"decision":"repair","field":"required_behaviors","defect":"The behaviors omit the requested channel audio-state interaction."}`)
	if err != nil {
		t.Fatal(err)
	}
	if repair.Decision != ApplicationJobSpecificationReviewRepair ||
		repair.Field != ApplicationJobSpecificationRequiredBehaviorsField || repair.Defect == "" {
		t.Fatalf("repair review=%+v", repair)
	}

	invalid := []string{
		`{"decision":"accept","field":"objective"}`,
		`{"decision":"accept","defect":"Unwanted."}`,
		`{"decision":"repair","field":"objective"}`,
		`{"decision":"repair","field":"path","defect":"Wrong."}`,
		`{"decision":"repair","field":"objective","defect":" Wrong. "}`,
		`{"decision":"repair","field":"objective","defect":"Wrong.","path":"src/app.tsx"}`,
	}
	for _, raw := range invalid {
		if _, err := DecodeApplicationJobSpecificationReview(input, raw); err == nil {
			t.Fatalf("accepted invalid review %s", raw)
		}
	}
	if _, err := NewApplicationJobSpecificationReviewInput(
		applicationJobSpecificationTestInput(1), applicationJobSpecificationTestValue(), 4,
	); err == nil {
		t.Fatal("accepted an unbounded fourth review")
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
	review, err := DecodeApplicationJobSpecificationReview(reviewInput, `{"decision":"repair","field":"required_behaviors","defect":"Required behavior is not specific enough to execute."}`)
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
	for _, exact := range []string{review.Defect, retained.Objective, retained.RequiredBehaviors[0]} {
		if !strings.Contains(prompt, exact) {
			t.Fatalf("repair prompt omitted %q:\n%s", exact, prompt)
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
			review := applicationJobSpecificationRepairReview(
				t, authority, retained, field,
				"The field is not specific enough to implement and verify.",
			)
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
		"The objective is too vague to execute.",
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
	if _, err := NewApplicationJobSpecificationRepairInput(authority, retained, review, 3); err == nil {
		t.Fatal("accepted a third repair")
	}
	accepted := review
	accepted.Decision = ApplicationJobSpecificationReviewAccept
	accepted.Field = ""
	accepted.Defect = ""
	if _, err := NewApplicationJobSpecificationRepairInput(authority, retained, accepted, 1); err == nil {
		t.Fatal("constructed repair from accepted review")
	}
}

func applicationJobSpecificationRepairReview(
	t testing.TB,
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
	defect string,
) ApplicationJobSpecificationReview {
	t.Helper()
	input, err := NewApplicationJobSpecificationReviewInput(authority, retained, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]string{
		"decision": string(ApplicationJobSpecificationReviewRepair),
		"field":    string(field), "defect": defect,
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
