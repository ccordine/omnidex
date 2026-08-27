package worker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestObjectiveCompletionAllowsExactMaximumOrderedRoleplayRound(t *testing.T) {
	t.Parallel()
	responses := make([]queue.RoleplayResponseCompletion, roleplay.MaxSceneParticipants)
	for index := range responses {
		responses[index] = queue.RoleplayResponseCompletion{
			Position: index,
			CharacterID: model.RoleplayCharacterID(
				fmt.Sprintf("rpc_%032x", index+1),
			),
			Output: strings.Repeat("a", roleplay.MaxNarrativeResponseBytes),
		}
	}
	output := queue.RenderRoleplayResponseRound(responses)
	if len(output) <= maxObjectiveOutputBytes {
		t.Fatalf("test aggregate bytes=%d want >%d", len(output), maxObjectiveOutputBytes)
	}
	result := objectiveTurnResult{
		ObjectiveID: "roleplay-round", RequirementID: "roleplay-round-result",
		InstructionSHA256: strings.Repeat("a", 64),
		Kind:              assemblyline.ObjectiveKindStory,
		Output:            output, RoleplayResponses: responses, Complete: true,
	}
	prepared, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil || prepared != output || len(records) != 0 {
		t.Fatalf("prepared bytes=%d records=%d error=%v", len(prepared), len(records), err)
	}
	result.Output += "x"
	if _, _, err := prepareObjectiveTurnCompletion(result); err == nil ||
		!strings.Contains(err.Error(), "differs") {
		t.Fatalf("mismatched aggregate error=%v", err)
	}
}

func TestObjectiveCompletionPreparesExactCitationsForAtomicCommit(t *testing.T) {
	t.Parallel()

	citation := mustObjectiveEvidence(
		t, "R01", "func ReadThing() string", "repository_symbol", "pack-17#symbol-31",
	)
	citation.SourceSHA256 = strings.Repeat("d", 64)
	result := objectiveTurnResult{
		ObjectiveID: "objective-17", RequirementID: "requirement-17",
		InstructionSHA256: strings.Repeat("a", 64), Kind: assemblyline.ObjectiveKindRepositoryRead,
		Citations: []objectiveEvidence{citation}, Output: "ReadThing owns this behavior.",
		Complete: true,
	}
	output, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil {
		t.Fatal(err)
	}
	wantOutput := "ReadThing owns this behavior.\n\nSources:\n- [R01] repository_symbol:pack-17#symbol-31 (source_sha256:" + citation.SourceSHA256 + ")"
	if output != wantOutput {
		t.Fatalf("rendered output=%q want %q", output, wantOutput)
	}
	if len(records) != 1 {
		t.Fatalf("persisted records=%d want 1", len(records))
	}
	want := evidence.Record{
		Kind: evidence.KindObjectiveCitation, SourceType: citation.SourceType,
		SourceRef: citation.SourceRef, Excerpt: citation.Capsule.Text,
		Summary: "Objective objective-17 cited evidence capsule R01.", Hash: citation.SourceSHA256,
		Confidence: 1, SupportsClaims: []string{"requirement-17"},
		Metadata: map[string]any{
			"capsule_id": "R01", "instruction_sha256": strings.Repeat("a", 64),
			"objective_id": "objective-17", "objective_kind": "repository_read",
			"requirement_id": "requirement-17", "projection_sha256": citation.SHA256,
			"source_sha256": strings.Repeat("d", 64),
		},
	}
	if !reflect.DeepEqual(records[0], want) {
		t.Fatalf("persisted record=%#v want %#v", records[0], want)
	}
}

func TestObjectiveEvidenceRejectsMalformedSourceDigest(t *testing.T) {
	t.Parallel()

	citation := mustObjectiveEvidence(t, "W01", "Exact evidence.", "web_document", "https://example.test")
	citation.SourceSHA256 = "malformed"
	if err := validateObjectiveEvidence(citation); err == nil {
		t.Fatal("malformed authoritative source digest was accepted")
	}
}

func TestObjectiveCompletionPreparesGroundedEvidenceWithoutSidecarWriter(t *testing.T) {
	t.Parallel()

	citation := mustObjectiveEvidence(t, "W01", "Current fact.", "web_document", "https://example.test/fact")
	citation.ParagraphMask = 1
	result := objectiveTurnResult{
		ObjectiveID: "objective-18", RequirementID: "requirement-18",
		InstructionSHA256: strings.Repeat("b", 64), Kind: assemblyline.ObjectiveKindExternalAnswer,
		Citations:         []objectiveEvidence{citation},
		Output:            "A grounded answer. [1]\n\nSources:\n[1] Fact — https://example.test/fact",
		CitationsRendered: true, Complete: true,
	}
	output, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil || output == "" || len(records) != 1 {
		t.Fatalf("prepared output=%q records=%d err=%v", output, len(records), err)
	}
}

func TestExternalCitationPersistenceKeepsExactParagraphClaimsSeparate(t *testing.T) {
	first := mustObjectiveEvidence(t, "W01", "First evidence.", "web_document", "https://first.test")
	first.ParagraphMask = 1 << 0
	first.Truncated = true
	second := mustObjectiveEvidence(t, "W02", "Second evidence.", "web_document", "https://second.test")
	second.ParagraphMask = 1 << 1
	result := objectiveTurnResult{
		ObjectiveID: "objective-web", RequirementID: "requirement-web",
		InstructionSHA256: strings.Repeat("e", 64), Kind: assemblyline.ObjectiveKindExternalAnswer,
		Citations:         []objectiveEvidence{first, second},
		Output:            "First. [1]\n\nSecond. [2]\n\nSources:\n[1] First — https://first.test\n[2] Second — https://second.test",
		CitationsRendered: true, Complete: true,
	}
	_, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || !reflect.DeepEqual(records[0].SupportsClaims, []string{"requirement-web#paragraph-1"}) ||
		!reflect.DeepEqual(records[1].SupportsClaims, []string{"requirement-web#paragraph-2"}) {
		t.Fatalf("claim/source bindings crossed: %#v", records)
	}
	if records[0].Metadata["source_truncated"] != true ||
		records[0].Metadata["source_observed_at"] != first.ObservedAt.Format(time.RFC3339Nano) {
		t.Fatalf("web freshness/truncation authority was lost: %#v", records[0].Metadata)
	}
}

func TestObjectiveCitationSelectionPreservesProviderProjection(t *testing.T) {
	t.Parallel()

	first := mustObjectiveEvidence(t, "R01", "First exact capsule.", "repository_symbol", "pack#one")
	second := mustObjectiveEvidence(t, "R02", "Second exact capsule.", "repository_symbol", "pack#two")
	selected, err := selectObjectiveCitations([]objectiveEvidence{first, second}, []string{"R02"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected, []objectiveEvidence{second}) {
		t.Fatalf("selected citations=%#v", selected)
	}
}

func TestObjectiveResultRequiresCitationsOnlyForGroundedKinds(t *testing.T) {
	t.Parallel()

	citation := mustObjectiveEvidence(t, "R01", "Exact evidence.", "repository_symbol", "pack#symbol")
	base := objectiveTurnResult{
		ObjectiveID: "objective-19", RequirementID: "requirement-19",
		InstructionSHA256: strings.Repeat("c", 64), Output: "Complete.", Complete: true,
	}
	ungrounded := base
	ungrounded.Kind = assemblyline.ObjectiveKindAnswer
	ungrounded.Citations = []objectiveEvidence{citation}
	if _, err := renderObjectiveTurnOutput(ungrounded); err == nil {
		t.Fatal("ungrounded answer accepted a citation projection")
	}
	grounded := base
	grounded.Kind = assemblyline.ObjectiveKindRepositoryRead
	if _, err := renderObjectiveTurnOutput(grounded); err == nil {
		t.Fatal("repository answer completed without cited evidence")
	}
}
