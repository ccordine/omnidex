package assemblyline

import "testing"

func TestDatabaseJoinPathSelectionBindsOnlyOneCurrentOpaquePath(t *testing.T) {
	input := DatabaseJoinPathSelectionInput{
		EvidenceNeedID: "need-join", ExactNeed: "Associate each message with its recipient.",
		FromRelationID: "rel_messages", ToRelationID: "rel_people",
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
	_, err = RenderPortableJob(job)
	if err != nil {
		t.Fatalf("render raw join-path station: %v", err)
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
