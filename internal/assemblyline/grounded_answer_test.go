package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestGroundedAnswerParagraphLeavesKeepInventorySupportAndAuthorizationSeparate(t *testing.T) {
	t.Parallel()
	base := groundedAnswerFixture()
	inventoryInput := GroundedAnswerParagraphInventoryInput{
		ExactRequirement: base.ExactRequirement,
		Context:          base.Context,
		Evidence:         base.Evidence,
	}
	job, err := NewGroundedAnswerParagraphInventoryJob(inventoryInput)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkGroundedAnswerParagraphInventory {
		t.Fatalf("inventory kind=%q", job.Kind)
	}
	prompt, err := BuildGroundedAnswerParagraphInventoryPrompt(inventoryInput)
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range base.Evidence {
		if !strings.Contains(prompt, evidence.Text) || strings.Contains(prompt, evidence.ID) {
			t.Fatalf("inventory prompt exposed an evidence ID or lost evidence text: %s", prompt)
		}
	}
	const paragraph = "The dispatch interval controls invitation timing."
	inventory, err := DecodeGroundedAnswerParagraphInventory(
		inventoryInput, paragraph+"\n"+paragraph,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 2 {
		t.Fatalf("inventory candidates=%v", inventory.Candidates)
	}

	relationInput := GroundedAnswerParagraphEvidenceRelationInput{
		ParagraphText: paragraph,
		Evidence:      base.Evidence[0],
	}
	relationJob, err := NewGroundedAnswerParagraphEvidenceRelationJob(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	if relationJob.Kind != WorkGroundedAnswerParagraphEvidenceRelation {
		t.Fatalf("relation kind=%q", relationJob.Kind)
	}
	relationPrompt, err := BuildGroundedAnswerParagraphEvidenceRelationPrompt(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(relationPrompt, paragraph) ||
		!strings.Contains(relationPrompt, base.Evidence[0].Text) ||
		strings.Contains(relationPrompt, base.Evidence[0].ID) ||
		strings.Contains(relationPrompt, base.ExactRequirement) {
		t.Fatalf("pairwise relation prompt exceeded its exact authority: %s", relationPrompt)
	}
	relation, err := DecodeGroundedAnswerParagraphEvidenceRelationDecision(
		relationInput, string(GroundedEvidenceSupportsParagraph),
	)
	if err != nil || relation.Relation != GroundedEvidenceSupportsParagraph {
		t.Fatalf("relation=%+v err=%v", relation, err)
	}

	authorizationInput := GroundedAnswerParagraphAuthorizationInput{
		ExactRequirement: base.ExactRequirement,
		Context:          base.Context,
		ParagraphText:    paragraph,
		Evidence:         base.Evidence[:1],
	}
	authorizationJob, err := NewGroundedAnswerParagraphAuthorizationJob(authorizationInput)
	if err != nil {
		t.Fatal(err)
	}
	if authorizationJob.Kind != WorkGroundedAnswerParagraphAuthorization {
		t.Fatalf("authorization kind=%q", authorizationJob.Kind)
	}
	authorizationPrompt, err := BuildGroundedAnswerParagraphAuthorizationPrompt(authorizationInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authorizationPrompt, base.Evidence[0].Text) ||
		strings.Contains(authorizationPrompt, base.Evidence[1].Text) ||
		strings.Contains(authorizationPrompt, base.Evidence[0].ID) {
		t.Fatalf("authorization prompt did not receive only supporting evidence: %s", authorizationPrompt)
	}
	authorization, err := DecodeGroundedAnswerParagraphAuthorizationDecision(
		authorizationInput, string(GroundedParagraphResponsiveAndFullySupported),
	)
	if err != nil || authorization.Relation != GroundedParagraphResponsiveAndFullySupported {
		t.Fatalf("authorization=%+v err=%v", authorization, err)
	}
}

func TestGroundedAnswerParagraphDecodersRejectStructuredCompositeAndCitationResults(t *testing.T) {
	t.Parallel()
	base := groundedAnswerFixture()
	inventoryInput := GroundedAnswerParagraphInventoryInput{
		ExactRequirement: base.ExactRequirement, Context: base.Context, Evidence: base.Evidence,
	}
	for _, raw := range []string{
		`{"paragraphs":["answer"]}`,
		`["answer"]`,
		`"answer"`,
		"```\nanswer\n```",
		"answer [1]",
	} {
		if _, err := DecodeGroundedAnswerParagraphInventory(inventoryInput, raw); err == nil {
			t.Fatalf("structured or cited inventory result accepted: %q", raw)
		}
	}
	relationInput := GroundedAnswerParagraphEvidenceRelationInput{
		ParagraphText: "Answer.", Evidence: base.Evidence[0],
	}
	for _, raw := range []string{
		"supports", "SUPPORTS_PARAGRAPH\ncomment", `{"relation":"SUPPORTS_PARAGRAPH"}`,
	} {
		if _, err := DecodeGroundedAnswerParagraphEvidenceRelationDecision(relationInput, raw); err == nil {
			t.Fatalf("invalid relation accepted: %q", raw)
		}
	}
}

func TestGroundedAnswerParagraphInventorySupportsExplicitAbsence(t *testing.T) {
	t.Parallel()
	base := groundedAnswerFixture()
	input := GroundedAnswerParagraphInventoryInput{
		ExactRequirement: base.ExactRequirement, Context: base.Context, Evidence: base.Evidence,
	}
	inventory, err := DecodeGroundedAnswerParagraphInventory(
		input, GroundedAnswerNoParagraphCandidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Candidates == nil || len(inventory.Candidates) != 0 {
		t.Fatalf("absence inventory=%#v", inventory)
	}
}

func TestGroundedAnswerParagraphRejectsFilesystemIdentityAtResultBoundary(t *testing.T) {
	t.Parallel()
	base := groundedAnswerFixture()
	input := GroundedAnswerParagraphInventoryInput{
		ExactRequirement:   base.ExactRequirement,
		Context:            base.Context,
		Evidence:           base.Evidence,
		KnownArtifactPaths: []string{"internal/private/secret_owner.go"},
	}
	for name, candidate := range map[string]string{
		"posix absolute": "/private/file.go contains the setting.",
		"traversal":      "../private contains the setting.",
		"windows":        `C:\private\file.go contains the setting.`,
		"known basename": "secret_owner.go contains the setting.",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeGroundedAnswerParagraphInventory(input, candidate); err == nil {
				t.Fatalf("path-bearing grounded answer paragraph was accepted: %q", candidate)
			}
		})
	}
}

func TestGroundedAnswerArtifactProvenanceRemainsOutsideInventoryPrompt(t *testing.T) {
	t.Parallel()
	base := groundedAnswerFixture()
	input := GroundedAnswerParagraphInventoryInput{
		ExactRequirement:   base.ExactRequirement,
		Context:            base.Context,
		Evidence:           base.Evidence,
		KnownArtifactPaths: []string{"internal/private/secret_owner.go"},
	}
	job, err := NewGroundedAnswerParagraphInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"internal/private/secret_owner.go", "secret_owner.go"} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("grounded answer prompt exposed code-owned artifact provenance %q: %s", hidden, prompt)
		}
	}
	if !strings.Contains(string(job.Payload), "internal/private/secret_owner.go") {
		t.Fatalf("portable payload lost code-owned artifact provenance: %s", job.Payload)
	}
}

func TestAssembleGroundedAnswerDecisionPreservesParagraphAndEvidenceOrder(t *testing.T) {
	t.Parallel()
	input := groundedAnswerFixture()
	decision, err := AssembleGroundedAnswerDecision(input, []GroundedAnswerParagraph{
		{Text: "The dispatch interval controls invitation timing.", EvidenceIDs: []string{"E17", "E31"}},
		{Text: "The scheduler reads that interval.", EvidenceIDs: []string{"E31"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Text != "The dispatch interval controls invitation timing.\n\nThe scheduler reads that interval." ||
		!reflect.DeepEqual(decision.EvidenceIDs, []string{"E17", "E31"}) {
		t.Fatalf("decision=%#v", decision)
	}
	if _, err := AssembleGroundedAnswerDecision(input, nil); err == nil {
		t.Fatal("zero accepted paragraphs were silently assembled")
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
