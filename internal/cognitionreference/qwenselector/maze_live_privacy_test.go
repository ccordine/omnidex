package qwenselector_test

import "testing"

func TestMazePrivacyLexicalCheckAllowsBenignEmbeddedSubstrings(t *testing.T) {
	t.Parallel()
	benign := "At least one actionable route has movement-worthy goalposts and toolmakers."
	if token, forbidden := findForbiddenMazeMechanic(benign); forbidden {
		t.Fatalf("benign semantic text false-matched forbidden token %q", token)
	}
}

func TestMazePrivacyLexicalCheckRejectsStandaloneMechanics(t *testing.T) {
	t.Parallel()
	for _, mechanic := range liveMazeForbiddenMechanicTokens {
		mechanic := mechanic
		t.Run(mechanic, func(t *testing.T) {
			t.Parallel()
			if token, forbidden := findForbiddenMazeMechanic("public " + mechanic + " value"); !forbidden || token != mechanic {
				t.Fatalf("standalone mechanic %q was not rejected: token=%q forbidden=%t", mechanic, token, forbidden)
			}
		})
	}
}

func TestMazePrivacyLexicalCheckTreatsPunctuationAsBoundary(t *testing.T) {
	t.Parallel()
	for _, text := range []string{`{"field":"east"}`, "tool-call", "operation_id"} {
		if _, forbidden := findForbiddenMazeMechanic(text); !forbidden {
			t.Fatalf("punctuation-delimited mechanic was not rejected: %q", text)
		}
	}
}
