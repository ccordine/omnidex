package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestObjectiveInstructionProjectionBindsQualifiedAndCurrentTreeIdentities(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{
		"internal/private/owner.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := "Compare owner.go with /etc/passwd."
	authority, err := bindObjectiveModelInstruction(turnAuthority{
		Instruction: raw, ChannelMode: model.ChannelModeAssistant,
	}, provenance)
	if err != nil {
		t.Fatal(err)
	}
	if authority.Instruction != raw || len(authority.ModelArtifactIdentities) != 2 ||
		!strings.Contains(authority.ModelInstruction, "ARTIFACT_1") ||
		!strings.Contains(authority.ModelInstruction, "ARTIFACT_2") {
		t.Fatalf("projected authority=%#v", authority)
	}
	for _, forbidden := range []string{"owner.go", "/etc/passwd"} {
		if strings.Contains(authority.ModelInstruction, forbidden) {
			t.Fatalf("model instruction leaked %q: %q", forbidden, authority.ModelInstruction)
		}
	}
	restored, err := restoreObjectiveModelText(
		authority, "test result", "ARTIFACT_1 differs from ARTIFACT_2.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if restored != "internal/private/owner.go differs from /etc/passwd." {
		t.Fatalf("restored=%q", restored)
	}
}

func TestRoleplayResearchProjectionRemovesCommandSyntaxAndPaths(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{"notes/private.txt"})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := bindObjectiveModelInstruction(turnAuthority{
		Instruction:       `/research "Compare private.txt with /etc/passwd"`,
		ChannelMode:       model.ChannelModeRoleplay,
		RoleplayInputKind: roleplay.SimulationTurnExternalCommand,
	}, provenance)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(authority.ModelInstruction, "/research") ||
		strings.Contains(authority.ModelInstruction, "private.txt") ||
		strings.Contains(authority.ModelInstruction, "/etc/passwd") {
		t.Fatalf("roleplay research projection leaked command/path: %q", authority.ModelInstruction)
	}
}

func TestObjectiveRawCandidateBoundaryRejectsQualifiedAndKnownPaths(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{"internal/private/owner.go"})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{"Read /etc/passwd.", "owner.go owns it."} {
		if err := validateObjectiveRawCandidatePathBoundary(
			assemblyline.WorkConversationResponse, candidate, provenance,
		); err == nil {
			t.Fatalf("path-bearing candidate %q was accepted", candidate)
		}
	}
	if err := validateObjectiveRawCandidatePathBoundary(
		assemblyline.WorkConversationResponse, "ARTIFACT_1 owns it.", provenance,
	); err != nil {
		t.Fatalf("bound token was rejected: %v", err)
	}
	if err := validateObjectiveRawCandidatePathBoundary(
		assemblyline.WorkDatabaseQueryFilterValue, "/typed/database/value", provenance,
	); err != nil {
		t.Fatalf("typed database scalar was treated as filesystem authority: %v", err)
	}
}
