package scrumcardllm

import (
	"encoding/json"
	"testing"
)

func TestJobMetadataRoundTripRequiresActionModel(t *testing.T) {
	raw, err := JobMetadata(7, "card-1", ActionTagsSuggest, "coach-model", "", TicketRequest{})
	if err != nil {
		t.Fatalf("encode metadata: %v", err)
	}
	parsed, err := ParseMetadata(raw)
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	if parsed.ProjectID != 7 || parsed.CardID != "card-1" || parsed.CoachModel != "coach-model" {
		t.Fatalf("unexpected parsed metadata: %#v", parsed)
	}
	if _, err := JobMetadata(7, "card-1", ActionTagsSuggest, "", "", TicketRequest{}); err == nil {
		t.Fatal("expected missing coach model to fail")
	}
	if _, err := JobMetadata(7, "card-1", ActionCardTicket, "", "", TicketRequest{}); err == nil {
		t.Fatal("expected missing ticket model to fail")
	}
}

func TestParseMetadataRejectsCoercionUnknownFieldsAndMalformedTicket(t *testing.T) {
	for _, raw := range []string{
		`{"source":"scrum_card_llm","project_id":"7","scrum_card_id":"card-1","action":"tags_suggest","coach_model":"model"}`,
		`{"source":"scrum_card_llm","project_id":7,"scrum_card_id":"card-1","action":"tags_suggest","coach_model":"model","unknown":true}`,
		`{"source":"scrum_card_llm","project_id":7,"scrum_card_id":"card-1","action":"card_ticket","ticket_model":"model"}`,
		`{"source":"scrum_card_llm","project_id":7,"scrum_card_id":"card-1","action":"card_ticket","ticket_model":"model","ticket_request":{"iterate":true}}`,
	} {
		if _, err := ParseMetadata(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected invalid metadata to fail: %s", raw)
		}
	}
}

func TestParseJobReferenceAllowsFailedActionMetadataButRejectsUnknownFields(t *testing.T) {
	reference, err := ParseJobReference(json.RawMessage(`{
		"source":"scrum_card_llm",
		"project_id":42,
		"scrum_card_id":"card-42",
		"action":"tags_suggest"
	}`))
	if err != nil {
		t.Fatalf("ParseJobReference() error: %v", err)
	}
	if reference.ProjectID != 42 || reference.CardID != "card-42" {
		t.Fatalf("ParseJobReference()=%#v", reference)
	}
	if _, err := ParseJobReference(json.RawMessage(`{
		"source":"scrum_card_llm",
		"project_id":42,
		"scrum_card_id":"card-42",
		"unknown":true
	}`)); err == nil {
		t.Fatal("ParseJobReference() accepted an unknown field")
	}
}
