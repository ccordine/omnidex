package webresearch

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestValidateCompletionArtifactRejectsEveryIndependentCitationMutation(t *testing.T) {
	document := documentFixture("https://authority.example/source", "Authority", "Exact acquired authority.")
	evidence := evidenceFromDocuments([]websearch.Document{document})
	projected := []ProjectedEvidence{{
		EvidenceID: evidence[0].ID, CandidateID: evidence[0].CandidateID,
		Title: evidence[0].Title, Snippet: evidence[0].Snippet, Content: evidence[0].Content,
	}}
	artifact, err := buildArtifact(GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "The acquired authority supports the claim.", EvidenceIDs: []EvidenceID{evidence[0].ID},
	}}}, projected, evidence, Config{MaxSynthesisParagraphs: 4, MaxSynthesisParagraphBytes: 2 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCompletionArtifact(artifact, evidence); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*Artifact){
		"number":      func(value *Artifact) { value.Sources[0].Number++ },
		"evidence":    func(value *Artifact) { value.Sources[0].EvidenceID = EvidenceID("evidence_" + strings.Repeat("f", 64)) },
		"candidate":   func(value *Artifact) { value.Sources[0].CandidateID = "candidate_forged" },
		"document":    func(value *Artifact) { value.Sources[0].DocumentID = "document_forged" },
		"title":       func(value *Artifact) { value.Sources[0].Title = "Forged title" },
		"url":         func(value *Artifact) { value.Sources[0].URL = "https://forged.example" },
		"source hash": func(value *Artifact) { value.Sources[0].ContentSHA256 = strings.Repeat("f", 64) },
		"observed at": func(value *Artifact) { value.Sources[0].ObservedAt = value.Sources[0].ObservedAt.Add(1) },
		"truncated":   func(value *Artifact) { value.Sources[0].Truncated = !value.Sources[0].Truncated },
		"rendered":    func(value *Artifact) { value.Rendered += " forged" },
		"self hash":   func(value *Artifact) { value.SHA256 = strings.Repeat("f", 64) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := cloneArtifact(artifact)
			mutate(&changed)
			if err := ValidateCompletionArtifact(changed, evidence); err == nil {
				t.Fatal("independently mutated completion authority was accepted")
			}
		})
	}
}
