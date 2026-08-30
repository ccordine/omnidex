package assemblyline

import (
	"strings"
	"testing"
)

func TestDownstreamSemanticPromptsHideObjectiveContextProvenance(t *testing.T) {
	t.Parallel()
	context := objectiveContextPromptProjectionFixture()
	queryIntent, _, _ := databaseQueryIntentFixture(t)

	tests := []struct {
		name string
		job  func() (PortableJob, error)
	}{
		{name: "grounded answer", job: func() (PortableJob, error) {
			base := groundedAnswerFixture()
			return NewGroundedAnswerParagraphInventoryJob(GroundedAnswerParagraphInventoryInput{
				ExactRequirement: base.ExactRequirement,
				Context:          context,
				Evidence:         base.Evidence,
			})
		}},
		{name: "roleplay grounded response", job: func() (PortableJob, error) {
			input := roleplayGroundedFixture()
			input.Context = context
			return NewRoleplayGroundedParagraphInventoryJob(input)
		}},
		{name: "web relevance", job: func() (PortableJob, error) {
			input := webRelevanceFixture()
			input.Context = context
			return NewWebRelevanceRelationJob(WebRelevanceRelationInput{
				ExactQuestion: input.ExactQuestion, Context: input.Context,
				Candidate: input.Candidates[0],
			})
		}},
		{name: "web grounded synthesis", job: func() (PortableJob, error) {
			base := webSynthesisFixture()
			base.Context = context
			return NewWebSynthesisParagraphInventoryJob(base)
		}},
		{name: "database join path", job: func() (PortableJob, error) {
			return NewDatabaseJoinPathSelectionJob(DatabaseJoinPathSelectionInput{
				EvidenceNeedID: "need-join", ExactNeed: "Associate the event with its owner.",
				Context: context, FromRelationID: "rel_events", ToRelationID: "rel_people",
				Candidates: []DatabaseJoinPathCandidate{
					{PathID: "path_owner", Descriptor: `[{"foreign_key":"events.owner_id"}]`},
					{PathID: "path_actor", Descriptor: `[{"foreign_key":"events.actor_id"}]`},
				},
			})
		}},
		{name: "database query intent", job: func() (PortableJob, error) {
			input := queryIntent
			input.Context = context
			state := NewDatabaseQueryIntentLeafState(input)
			state.FromRelationID = input.SchemaProjection.Relations[0].ID
			return NewDatabaseQueryShapeJob(state)
		}},
		{name: "database schema selection", job: func() (PortableJob, error) {
			input := databaseSchemaSelectionFixture()
			input.Context = context
			inventoryInput, err := ProjectDatabaseSchemaRelationInventoryInput(input)
			if err != nil {
				return PortableJob{}, err
			}
			return NewDatabaseSchemaRelationInventoryJob(inventoryInput)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job, err := test.job()
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			for _, visible := range []string{context.Capsules[0].Content} {
				if !strings.Contains(prompt, visible) {
					t.Fatalf("prompt lost model-visible context %q: %s", visible, prompt)
				}
			}
			for _, hidden := range objectiveContextPromptHiddenSentinels(context) {
				if strings.Contains(prompt, hidden) {
					t.Fatalf("prompt leaked code-owned objective context provenance %q: %s", hidden, prompt)
				}
				if !strings.Contains(string(job.Payload), hidden) {
					t.Fatalf("portable payload lost code-owned objective context provenance %q: %s", hidden, job.Payload)
				}
			}
		})
	}
}

func objectiveContextPromptProjectionFixture() ObjectiveContext {
	capsule := "MINIFIED_CAPSULE_VISIBLE_SENTINEL"
	feedback := "EXACT_REPLAN_FEEDBACK_HIDDEN_SENTINEL"
	return ObjectiveContext{
		Capsules: []ObjectiveContextCapsule{{
			Sources: []ObjectiveContextSource{{
				Namespace:     "prompt-leak-sentinel",
				CandidateID:   "CTX_987654",
				ContentSHA256: ExactObjectiveContextSHA("HIDDEN_SOURCE_BYTES_SENTINEL"),
			}},
			Content:       capsule,
			ContentSHA256: ExactObjectiveContextSHA(capsule),
		}},
		ReplanAuthority: &ObjectiveReplanAuthority{
			JobID:          7654321,
			Generation:     222,
			Feedback:       feedback,
			FeedbackSHA256: ExactObjectiveContextSHA(feedback),
		},
	}
}

func objectiveContextPromptHiddenSentinels(context ObjectiveContext) []string {
	return []string{
		context.Capsules[0].Sources[0].Namespace,
		context.Capsules[0].Sources[0].CandidateID,
		context.Capsules[0].Sources[0].ContentSHA256,
		context.Capsules[0].ContentSHA256,
		context.ReplanAuthority.Feedback,
		context.ReplanAuthority.FeedbackSHA256,
		`"job_id":7654321`,
		`"generation":222`,
	}
}
