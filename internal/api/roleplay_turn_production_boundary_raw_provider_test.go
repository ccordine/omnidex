package api

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRoleplayProductionBoundaryProviderReturnsOnlyRawStationLeaves(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, prompt, response string
		kind                   assemblyline.WorkKind
		terminal               bool
	}{
		{
			name: "context term remains",
			prompt: "Answer one semantic coverage relation: does the exact current instruction\n" +
				"ACCEPTED RETRIEVAL CONCEPTS:\n(none)",
			response: assemblyline.ContextTermRemains,
			kind:     assemblyline.WorkContextSearchTermCoverage,
		},
		{
			name:     "context term",
			prompt:   "Return exactly one concise retrieval concept",
			response: roleplayBoundarySearchTerm,
			kind:     assemblyline.WorkContextSearchTerm,
		},
		{
			name: "context terms complete",
			prompt: "Answer one semantic coverage relation: does the exact current instruction\n" +
				"ACCEPTED RETRIEVAL CONCEPT 1:\n" + roleplayBoundarySearchTerm,
			response: assemblyline.ContextNoUncoveredTerm,
			kind:     assemblyline.WorkContextSearchTermCoverage,
		},
		{
			name: "context relevance",
			prompt: "CONTEXT_RELEVANCE_AUTHORITY:\n" +
				`{"candidates":[{"candidate_id":"CTX_3"}],"accepted_candidate_ids":[]}`,
			response: "CTX_3",
			kind:     assemblyline.WorkContextRelevanceSelection,
		},
		{
			name: "context minification", prompt: "CONTEXT_MINIFICATION_JSON:\n{}",
			response: "Mara is defending the archive.", kind: assemblyline.WorkContextMinification,
		},
		{
			name: "conversation response", prompt: "ROLEPLAY_IDENTITY_JSON:\n{}",
			response: roleplayBoundaryReply, kind: assemblyline.WorkConversationResponse,
		},
		{
			name: "ongoing action", prompt: "ROLEPLAY_ONGOING_ACTION_JSON:\n{}",
			response: roleplayBoundaryAction, kind: assemblyline.WorkRoleplayOngoingAction,
		},
		{
			name: "canon fact remains",
			prompt: "Answer one semantic coverage relation: does the exact current contribution\n" +
				"ACCEPTED CURRENT-CONTRIBUTION FACTS:\n(none)",
			response: assemblyline.RoleplayCanonFactRemains,
			kind:     assemblyline.WorkRoleplayCanonFactCoverage,
		},
		{
			name:     "canon fact",
			prompt:   "Return exactly one durable fictional fact established by the exact current contribution",
			response: roleplayBoundaryFact, kind: assemblyline.WorkRoleplayCanonFact,
		},
		{
			name: "canon facts complete",
			prompt: "Answer one semantic coverage relation: does the exact current contribution\n" +
				"ACCEPTED CURRENT-CONTRIBUTION FACT 1:\n" + roleplayBoundaryFact,
			response: assemblyline.RoleplayNoUncoveredCanonFact,
			kind:     assemblyline.WorkRoleplayCanonFactCoverage, terminal: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, kind, terminal, err := roleplayBoundaryRawResponse(test.prompt)
			if err != nil {
				t.Fatal(err)
			}
			if response != test.response || kind != test.kind || terminal != test.terminal {
				t.Fatalf(
					"response=%q kind=%q terminal=%t want %q/%q/%t",
					response, kind, terminal, test.response, test.kind, test.terminal,
				)
			}
			if json.Valid([]byte(response)) {
				t.Fatalf("provider response is structured JSON: %q", response)
			}
		})
	}
}

func TestRoleplayProductionBoundaryProviderRejectsUnknownAggregateEnvelope(t *testing.T) {
	t.Parallel()
	if _, _, _, err := roleplayBoundaryRawResponse(
		`{"schema":"omnidex.roleplay-canon-extraction.v1","facts":[]}`,
	); err == nil {
		t.Fatal("retired structured aggregate envelope was accepted")
	}
}
