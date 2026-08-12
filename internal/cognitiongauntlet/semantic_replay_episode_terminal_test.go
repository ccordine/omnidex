package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestSemanticReplayBindsEmbeddedEpisodeTerminalPayload(t *testing.T) {
	revision := cognition.WorldRevision{
		EpisodeID: cognition.EpisodeID("episode-" + strings.Repeat("a", 64)),
		Number:    2, SHA256: strings.Repeat("b", 64),
	}
	outcome := Outcome{Terminal: true, GoalSatisfied: true, PublicOutcome: "done"}
	traceSHA := strings.Repeat("c", 64)
	newEntry := func(value oracleTerminalTrace) TraceEntry {
		payload, err := traceJSONObject(value)
		if err != nil {
			t.Fatal(err)
		}
		copy := revision
		return TraceEntry{
			Kind: TraceTerminal, ID: "terminal-" + traceSHA,
			Revision: &copy, Payload: payload,
		}
	}
	valid := oracleTerminalTrace{
		Revision: revision, PublicOutcome: outcome.PublicOutcome,
		GoalSatisfied: outcome.GoalSatisfied,
	}
	if err := validateSemanticEpisodeTerminal(newEntry(valid), revision, outcome, traceSHA); err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name   string
		mutate func(*oracleTerminalTrace, *TraceEntry)
	}{
		{"payload_revision", func(value *oracleTerminalTrace, _ *TraceEntry) {
			value.Revision.Number--
		}},
		{"payload_outcome", func(value *oracleTerminalTrace, _ *TraceEntry) {
			value.PublicOutcome = "other"
		}},
		{"payload_goal", func(value *oracleTerminalTrace, _ *TraceEntry) {
			value.GoalSatisfied = false
		}},
		{"entry_revision", func(_ *oracleTerminalTrace, entry *TraceEntry) {
			copy := *entry.Revision
			copy.Number--
			entry.Revision = &copy
		}},
		{"trace_id", func(_ *oracleTerminalTrace, entry *TraceEntry) {
			entry.ID = "terminal-" + strings.Repeat("d", 64)
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			value := valid
			entry := newEntry(value)
			mutation.mutate(&value, &entry)
			if mutation.name[:7] == "payload" {
				entry = newEntry(value)
			}
			if validateSemanticEpisodeTerminal(entry, revision, outcome, traceSHA) == nil {
				t.Fatal("semantic replay accepted a forged embedded terminal trace")
			}
		})
	}
}
