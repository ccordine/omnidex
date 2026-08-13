package webresearch

import "testing"

func TestObjectivePreservesExactNonBlankQuestionWhitespace(t *testing.T) {
	question := "  What is established?\n"
	objective := Objective{
		ID: "objective_exact_question", Question: question, InitialQuery: "established evidence",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}
	machine := newFixtureMachine(
		t, objective, &scriptedAcquisition{},
		&recordingTermsStation{}, &recordingRelevanceStation{}, &recordingSynthesisStation{}, 1_000,
	)
	if machine.objective.Question != question {
		t.Fatalf("question=%q want exact %q", machine.objective.Question, question)
	}
}

func TestObjectiveRejectsInvalidExactQuestionBytes(t *testing.T) {
	for name, question := range map[string]string{
		"blank":         " \n\t ",
		"nul":           "question\x00",
		"invalid UTF-8": string([]byte{0xff}),
	} {
		t.Run(name, func(t *testing.T) {
			objective := Objective{
				ID: "objective_invalid_question", Question: question, InitialQuery: "query",
				Acceptance: exactAcceptance(), Status: ObjectivePending,
			}
			if err := validateObjective(objective); err == nil {
				t.Fatal("invalid exact question was accepted")
			}
		})
	}
}
