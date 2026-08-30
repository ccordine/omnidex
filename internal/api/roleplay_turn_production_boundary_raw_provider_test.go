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
			name: "context relevance",
			prompt: "CONTEXT RELEVANCE RELATION AUTHORITY:\n" +
				`{"exact_instruction":"Continue.","candidate_content":"The bridge is raised."}`,
			response: assemblyline.ContextCandidateDirectlyRelevant,
			kind:     assemblyline.WorkContextRelevanceRelation,
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
			name: "canon candidate inventory",
			prompt: "Return one bounded source-ordered inventory of candidate durable fictional facts\n" +
				"ROLEPLAY CANON CANDIDATE INVENTORY AUTHORITY:\n",
			response: roleplayBoundaryFact,
			kind:     assemblyline.WorkRoleplayCanonFactInventory,
		},
		{
			name: "canon candidate authorization",
			prompt: "Answer one semantic entailment question: is the exact candidate a durable fictional fact\n" +
				"EXACT CANDIDATE FACT:\n" + roleplayBoundaryFact,
			response: assemblyline.RoleplayCanonFactEstablished,
			kind:     assemblyline.WorkRoleplayCanonFactCandidateAuthorization, terminal: true,
		},
		{
			name: "canon candidate relation",
			prompt: "Answer one pairwise semantic relation: do the candidate fact and the already accepted fact\n" +
				"CANDIDATE FACT:\nA.\nALREADY ACCEPTED FACT:\nB.",
			response: assemblyline.RoleplayCanonFactsDistinct,
			kind:     assemblyline.WorkRoleplayCanonFactCandidateRelation,
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
