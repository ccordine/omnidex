package roleplay

import (
	"strings"
	"testing"
)

func TestParseResearchCommandAcceptsOnlyCanonicalQuotedGrammar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		exact    string
		question string
	}{
		{exact: `/research "How do ocean currents redistribute heat?"`, question: "How do ocean currents redistribute heat?"},
		{exact: `/research "Why does bread need steam during its initial bake?"`, question: "Why does bread need steam during its initial bake?"},
		{exact: `/research "What does \"orbital resonance\" mean?"`, question: `What does "orbital resonance" mean?`},
	}
	for _, test := range tests {
		command, matched, err := ParseResearchCommand(test.exact)
		if err != nil {
			t.Fatalf("ParseResearchCommand(%q): %v", test.exact, err)
		}
		if !matched || command.Question != test.question || command.Exact != test.exact {
			t.Fatalf("ParseResearchCommand(%q)=(%+v,%t), want question %q", test.exact, command, matched, test.question)
		}
	}
}

func TestParseResearchCommandReservesMalformedResearchNamespace(t *testing.T) {
	t.Parallel()
	invalid := []string{
		`/research`,
		`/research ocean currents`,
		`/research  "ocean currents"`,
		`/research ""`,
		`/research " padded "`,
		`/research "line\nbreak"`,
		`/research "unterminated`,
		`/research "valid" trailing`,
		` /research "not at byte zero"`,
		`/research "` + strings.Repeat("x", MaxResearchQuestionBytes+1) + `"`,
	}
	for _, exact := range invalid {
		_, matched, err := ParseResearchCommand(exact)
		if exact == ` /research "not at byte zero"` {
			if matched || err != nil {
				t.Fatalf("non-command input was reserved: matched=%t err=%v", matched, err)
			}
			continue
		}
		if !matched || err == nil {
			t.Fatalf("malformed research command %q matched=%t err=%v", exact, matched, err)
		}
	}

	for _, exact := range []string{`Continue the scene.`, `/give rpc_123 item`, `/researcher "topic"`} {
		_, matched, err := ParseResearchCommand(exact)
		if matched || err != nil {
			t.Fatalf("unrelated input %q was claimed: matched=%t err=%v", exact, matched, err)
		}
	}
}
