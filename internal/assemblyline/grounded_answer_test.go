package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGroundedAnswerStationCarriesOneRequirementAndOpaqueEvidence(t *testing.T) {
	t.Parallel()

	input := groundedAnswerFixture()
	job, err := NewGroundedAnswerJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkGroundedAnswer {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{input.RequirementID, input.ExactRequirement, "E17", "dispatch interval"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt omitted %q:\n%s", required, prompt)
		}
	}
	assertExactObjectSchemaFields(t, schema, []string{"schema", "requirement_id", "text", "evidence_ids"})
	properties := schema["properties"].(map[string]any)
	textSchema := properties["text"].(map[string]any)
	if _, finiteGrammarBound := textSchema["maxLength"]; finiteGrammarBound {
		t.Fatalf("grounded-answer schema encodes the code-owned byte ceiling: %#v", textSchema)
	}
	assertExactJSONFields(t, reflect.TypeOf(input), []string{
		"requirement_id", "exact_requirement", "objective_context", "evidence",
	})
	assertExactJSONFields(t, reflect.TypeOf(GroundedEvidenceCapsule{}), []string{"id", "text"})
	assertExactJSONFields(t, reflect.TypeOf(GroundedAnswerDecision{}), []string{"schema", "requirement_id", "text", "evidence_ids"})

	payload := string(job.Payload)
	for _, forbidden := range []string{"path", "tool", "capabilit", "plan", "completion"} {
		if strings.Contains(strings.ToLower(payload), `"`+forbidden) {
			t.Fatalf("grounded-answer payload exposes forbidden field %q: %s", forbidden, payload)
		}
	}
}

func TestGroundedAnswerBindsRequirementAndEvidenceExactly(t *testing.T) {
	t.Parallel()

	input := groundedAnswerFixture()
	valid := GroundedAnswerDecision{
		Schema:        GroundedAnswerSchemaV1,
		RequirementID: input.RequirementID,
		Text:          "The dispatch interval controls invitation timing.",
		EvidenceIDs:   []string{"E17"},
	}
	if err := valid.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	invalidUTF8 := string([]byte{0xff})
	for name, mutate := range map[string]func(*GroundedAnswerDecision){
		"schema":             func(value *GroundedAnswerDecision) { value.Schema = "wrong" },
		"requirement":        func(value *GroundedAnswerDecision) { value.RequirementID = "R18" },
		"empty_text":         func(value *GroundedAnswerDecision) { value.Text = "" },
		"untrimmed_text":     func(value *GroundedAnswerDecision) { value.Text = " answer " },
		"nul_text":           func(value *GroundedAnswerDecision) { value.Text = "answer\x00" },
		"invalid_utf8_text":  func(value *GroundedAnswerDecision) { value.Text = invalidUTF8 },
		"oversized_text":     func(value *GroundedAnswerDecision) { value.Text = strings.Repeat("x", maxGroundedAnswerTextBytes+1) },
		"no_evidence":        func(value *GroundedAnswerDecision) { value.EvidenceIDs = nil },
		"unknown_evidence":   func(value *GroundedAnswerDecision) { value.EvidenceIDs = []string{"E99"} },
		"duplicate_evidence": func(value *GroundedAnswerDecision) { value.EvidenceIDs = []string{"E17", "E17"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.EvidenceIDs = append([]string(nil), valid.EvidenceIDs...)
			mutate(&candidate)
			if err := candidate.ValidateFor(input); err == nil {
				t.Fatalf("invalid decision %q accepted: %#v", name, candidate)
			}
		})
	}
}

func TestGroundedAnswerRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	invalidUTF8 := string([]byte{0xff})
	base := groundedAnswerFixture()
	tests := map[string]func(*GroundedAnswerInput){
		"empty_requirement_id": func(value *GroundedAnswerInput) { value.RequirementID = "" },
		"untrimmed_id":         func(value *GroundedAnswerInput) { value.RequirementID = " R17" },
		"multiline_id":         func(value *GroundedAnswerInput) { value.RequirementID = "R17\nR18" },
		"empty_requirement":    func(value *GroundedAnswerInput) { value.ExactRequirement = "" },
		"invalid_requirement":  func(value *GroundedAnswerInput) { value.ExactRequirement = invalidUTF8 },
		"oversized_requirement": func(value *GroundedAnswerInput) {
			value.ExactRequirement = strings.Repeat("x", maxGroundedRequirementBytes+1)
		},
		"rewritten_context_capsule": func(value *GroundedAnswerInput) {
			value.Context = minifiedObjectiveContext("Earlier answer.")
			value.Context.Capsules[0].Content = "Rewritten answer."
		},
		"no_evidence": func(value *GroundedAnswerInput) { value.Evidence = nil },
		"too_many_evidence": func(value *GroundedAnswerInput) {
			value.Evidence = make([]GroundedEvidenceCapsule, maxGroundedEvidenceCapsules+1)
			for index := range value.Evidence {
				value.Evidence[index] = GroundedEvidenceCapsule{ID: "E" + strings.Repeat("x", index+1), Text: "evidence"}
			}
		},
		"duplicate_evidence": func(value *GroundedAnswerInput) {
			value.Evidence[1].ID = value.Evidence[0].ID
		},
		"empty_evidence_text": func(value *GroundedAnswerInput) { value.Evidence[0].Text = " \n" },
		"nul_evidence_text":   func(value *GroundedAnswerInput) { value.Evidence[0].Text = "fact\x00" },
		"oversized_evidence": func(value *GroundedAnswerInput) {
			value.Evidence[0].Text = strings.Repeat("x", maxGroundedEvidenceTextBytes+1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := base
			input.Evidence = append([]GroundedEvidenceCapsule(nil), base.Evidence...)
			mutate(&input)
			if _, err := NewGroundedAnswerJob(input); err == nil {
				t.Fatalf("invalid input %q accepted: %#v", name, input)
			}
		})
	}
}

func TestGroundedAnswerDecodeRejectsExtraAndDuplicateState(t *testing.T) {
	t.Parallel()

	input := groundedAnswerFixture()
	valid := `{"schema":"omnidex.grounded-answer.v1","requirement_id":"R17","text":"Use the configured interval.","evidence_ids":["E17"]}`
	decision, err := DecodeGroundedAnswerDecision(input, valid)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RequirementID != input.RequirementID {
		t.Fatalf("decision=%#v", decision)
	}
	for _, raw := range []string{
		`{"schema":"omnidex.grounded-answer.v1","requirement_id":"R17","text":"Use it.","evidence_ids":["E17"],"action":"edit"}`,
		`{"schema":"omnidex.grounded-answer.v1","requirement_id":"R17","text":"Use it.","evidence_ids":["E17","E17"]}`,
		`{"schema":"omnidex.grounded-answer.v1","requirement_id":"R17","text":"first","text":"second","evidence_ids":["E17"]}`,
		`{"schema":"omnidex.grounded-answer.v1","Requirement_ID":"R17","text":"Use it.","evidence_ids":["E17"]}`,
		valid + `{}`,
	} {
		if _, err := DecodeGroundedAnswerDecision(input, raw); err == nil {
			t.Fatalf("invalid grounded answer accepted: %s", raw)
		}
	}
	if _, err := DecodeGroundedAnswerDecision(input, strings.Repeat("x", maxPortableCandidateBytes+1)); err == nil {
		t.Fatal("oversized grounded-answer candidate was parsed")
	}
	if schema, err := GroundedAnswerResponseSchema(GroundedAnswerInput{}); err == nil || schema != nil {
		t.Fatalf("invalid input produced response schema: %#v err=%v", schema, err)
	}
}

func groundedAnswerFixture() GroundedAnswerInput {
	return GroundedAnswerInput{
		RequirementID:    "R17",
		ExactRequirement: "Explain which setting controls invitation timing.",
		Evidence: []GroundedEvidenceCapsule{
			{ID: "E17", Text: "ClientDeliveryConfig declares dispatch interval."},
			{ID: "E31", Text: "InvitationScheduler reads the dispatch interval."},
		},
	}
}

func TestGroundedAnswerFixtureUTF8Assumption(t *testing.T) {
	t.Parallel()
	if !utf8.ValidString(groundedAnswerFixture().ExactRequirement) {
		t.Fatal("fixture is invalid UTF-8")
	}
	if _, err := json.Marshal(groundedAnswerFixture()); err != nil {
		t.Fatal(err)
	}
}
