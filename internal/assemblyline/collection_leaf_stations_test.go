package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestApplicationContextNeedLeavesUseCoverageAndOneRawQuestion(t *testing.T) {
	const request = "Exclude archived patients from the existing search."
	context, err := BootstrapApplicationContext(
		request, ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationContextNeedLeafInput{
		UserRequest: request, Context: context, AcceptedQuestions: []string{},
	}
	prompt, err := BuildApplicationContextNeedCoveragePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, request) || strings.Contains(prompt, context.Facts[0].SourceSHA256) {
		t.Fatalf("application context need projection crossed authority: %s", prompt)
	}
	if value, err := DecodeApplicationContextNeedCoverageLeaf(
		input, ApplicationContextNeedRemains,
	); err != nil || value != ApplicationContextNeedRemains {
		t.Fatalf("coverage=%q err=%v", value, err)
	}
	const question = "Which declaration owns archived-patient filtering?"
	if value, err := DecodeApplicationContextNeedQuestionLeaf(input, question); err != nil || value != question {
		t.Fatalf("question=%q err=%v", value, err)
	}
	input.AcceptedQuestions = []string{question}
	if _, err := DecodeApplicationContextNeedQuestionLeaf(input, question); err == nil {
		t.Fatal("duplicate application context question was accepted")
	}
	if _, err := DecodeApplicationContextNeedQuestionLeaf(input, `{"question":"unsafe"}`); err == nil {
		t.Fatal("JSON application context question was accepted")
	}
}

func TestRoleplayCanonFactLeavesUseOnlyCurrentContribution(t *testing.T) {
	input := RoleplayCanonFactLeafInput{
		Source: RoleplayCanonSource{
			Kind:                  RoleplayCanonSourceUserContribution,
			AttributedPersonaName: roleplay.NarratorPersonaName,
			ExactContribution:     "The bronze bell cracks.",
			PersonaKind:           roleplay.UserPersonaNarrator,
			ContributionKind:      roleplay.UserContributionNarration,
		},
		Context:       ObjectiveContext{Capsules: []ObjectiveContextCapsule{}},
		AcceptedFacts: []string{},
	}
	prompt, err := BuildRoleplayCanonFactPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.Source.ExactContribution) ||
		strings.Contains(prompt, `"accepted_facts"`) {
		t.Fatalf("roleplay canon fact prompt is not a raw leaf projection: %s", prompt)
	}
	const fact = "The bronze bell is cracked."
	if value, err := DecodeRoleplayCanonFactLeaf(input, fact); err != nil || value != fact {
		t.Fatalf("fact=%q err=%v", value, err)
	}
	input.AcceptedFacts = []string{fact}
	if _, err := DecodeRoleplayCanonFactLeaf(input, fact); err == nil {
		t.Fatal("duplicate roleplay canon fact was accepted")
	}
	if _, err := DecodeRoleplayCanonFactLeaf(input, `{"facts":["unsafe"]}`); err == nil {
		t.Fatal("JSON roleplay canon fact was accepted")
	}
	decision, err := AssembleRoleplayCanonExtractionDecision(
		input.extractionInput(), []string{fact},
	)
	if err != nil || len(decision.Facts) != 1 || decision.Facts[0] != fact {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
}
