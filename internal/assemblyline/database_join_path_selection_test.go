package assemblyline

import (
	"strings"
	"testing"
)

func TestDatabaseJoinPathSelectionBindsOnlyOneCurrentOpaquePath(t *testing.T) {
	input := DatabaseJoinPathSelectionInput{
		EvidenceNeedID: "need_hidden_1837", ExactNeed: "Associate each message with its recipient.",
		FromRelationID: "relation_hidden_from_4826", ToRelationID: "relation_hidden_to_5937",
		Candidates: []DatabaseJoinPathCandidate{
			{PathID: "path_sender", Descriptor: `[{"foreign_key":"messages.sender_id"}]`},
			{PathID: "path_recipient", Descriptor: `[{"foreign_key":"messages.recipient_id"}]`},
		},
	}
	job, err := NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkDatabaseJoinPathSelection {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatalf("render raw join-path station: %v", err)
	}
	for _, visible := range []string{
		input.ExactNeed,
		input.Candidates[0].PathID,
		"messages.sender_id",
		input.Candidates[1].PathID,
		"messages.recipient_id",
	} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("database join-path prompt omitted selection authority %q: %s", visible, prompt)
		}
	}
	for _, hidden := range []string{input.EvidenceNeedID, input.FromRelationID, input.ToRelationID} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("database join-path prompt exposed code-owned binding %q: %s", hidden, prompt)
		}
		if !strings.Contains(string(job.Payload), hidden) {
			t.Fatalf("database join-path payload lost code-owned binding %q: %s", hidden, job.Payload)
		}
	}
	decision, err := DecodeDatabaseJoinPathSelectionDecision(input,
		"path_recipient")
	if err != nil || decision.PathID != "path_recipient" {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
	for _, raw := range []string{
		"path_missing",
		`{"path_id":"path_sender","sql":"SELECT 1"}`,
		`"path_sender"`,
	} {
		if _, err := DecodeDatabaseJoinPathSelectionDecision(input, raw); err == nil {
			t.Fatalf("accepted invalid selection: %s", raw)
		}
	}
}
