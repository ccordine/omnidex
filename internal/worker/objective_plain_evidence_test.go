package worker

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestTurnAuthorityRetainsInstructionWithoutContentDerivedIdentity(t *testing.T) {
	t.Parallel()
	for _, instruction := range []string{"Explain this value exactly.\n", "Continue the scene.\tKeep the pause."} {
		authority, err := newTurnAuthority(model.Job{
			ID: 17, CurrentGeneration: 2, Pipeline: model.PipelineChat,
			Instruction: instruction, Metadata: []byte(`{"channel_id":"chat-test","channel_mode":"assistant"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if authority.Instruction != instruction || objectiveTurnID(authority) != "objective-17-2" {
			t.Fatalf("instruction or code-owned identity changed: %#v", authority)
		}
	}
}

func TestPlainObjectiveCompletionNeedsNoInstructionReceipt(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.ConversationObjectiveKind{
		assemblyline.ObjectiveKindAnswer, assemblyline.ObjectiveKindStory,
	} {
		result := objectiveTurnResult{
			ObjectiveID: "objective-17-2", RequirementID: "objective-17-2-requirement",
			Kind: kind, Output: "One accepted answer.\nAnother exact line.", Complete: true,
		}
		output, records, err := prepareObjectiveTurnCompletion(result)
		if err != nil {
			t.Fatal(err)
		}
		if output != result.Output || len(records) != 0 {
			t.Fatalf("plain completion manufactured output or evidence: %q / %#v", output, records)
		}
	}
}

func TestObjectiveCitationsPreserveExcerptAndSourceWithoutDuplicateTextHashes(t *testing.T) {
	t.Parallel()
	for _, excerpt := range []string{`{"value":12}`, `{"label":"ready","enabled":true}`} {
		citation, err := newObjectiveEvidence("DB-01", excerpt, "postgres_query", "database:fixture")
		if err != nil {
			t.Fatal(err)
		}
		if citation.DatabaseEvidenceID != 0 || citation.WebEvidenceID != 0 {
			t.Fatal("constructing an excerpt manufactured a recorded execution")
		}
		citation.DatabaseEvidenceID = 41
		citation.DatabaseRowEnd = 1
		citation.ObservedAt = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
		result := objectiveTurnResult{
			ObjectiveID: "objective-17-2", RequirementID: "objective-17-2-requirement",
			Kind: assemblyline.ObjectiveKindDatabaseRead, Output: "The query returned this value.",
			Citations: []objectiveEvidence{citation}, Complete: true,
		}
		output, records, err := prepareObjectiveTurnCompletion(result)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output, "sha256") ||
			!strings.Contains(output, citation.SourceRef) {
			t.Fatalf("citation did not render the actual source reference: %q", output)
		}
		if len(records) != 1 || records[0].Excerpt != excerpt || records[0].SourceRef != citation.SourceRef ||
			records[0].Metadata["database_evidence_id"] != "41" {
			t.Fatalf("citation lost actual evidence: %#v", records)
		}
		for _, key := range []string{"instruction_sha256", "projection_sha256", "source_sha256"} {
			if _, present := records[0].Metadata[key]; present {
				t.Errorf("citation retains duplicate text receipt %q", key)
			}
		}
	}
}

func TestDatabaseNeedIdentityBelongsToTheAcceptedRequirement(t *testing.T) {
	t.Parallel()
	for _, requirement := range []string{"objective-17-2-requirement", "objective-18-1-requirement"} {
		if got := objectiveDatabaseEvidenceNeedID(requirement); got != requirement+"-database-need" {
			t.Errorf("need ID %q is not owned by requirement %q", got, requirement)
		}
	}
}
