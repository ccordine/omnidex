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
			return NewGroundedAnswerTextJob(GroundedAnswerTextInput{
				ExactRequirement: base.ExactRequirement,
				Context:          context,
				Evidence:         base.Evidence,
			})
		}},
		{name: "repository grounded review", job: func() (PortableJob, error) {
			input := repositoryGroundedReviewFixture()
			input.Context = context
			return NewRepositoryGroundedIssueDetailJob(input)
		}},
		{name: "repository grounded correction", job: func() (PortableJob, error) {
			input := repositoryGroundedCorrectionFixture()
			input.Context = context
			return NewRepositoryGroundedCorrectionJob(input)
		}},
		{name: "roleplay grounded response", job: func() (PortableJob, error) {
			input := roleplayGroundedFixture()
			input.Context = context
			return NewRoleplayGroundedResponseTextJob(input)
		}},
		{name: "web search terms", job: func() (PortableJob, error) {
			input := webSearchTermsFixture()
			input.Context = context
			return NewWebSearchTermCoverageJob(WebSearchTermLeafInput{
				ExactQuestion: input.ExactQuestion, Context: input.Context,
				AttemptedQueries: input.AttemptedQueries, AcceptedTerms: []string{},
				MaxTerms: input.MaxTerms, MaxTermBytes: input.MaxTermBytes,
			})
		}},
		{name: "web relevance", job: func() (PortableJob, error) {
			input := webRelevanceFixture()
			input.Context = context
			return NewWebRelevanceRelationJob(WebRelevanceRelationInput{
				ExactQuestion: input.ExactQuestion, Context: input.Context,
				Candidate: input.Candidates[0],
			})
		}},
		{name: "web claim evidence review", job: func() (PortableJob, error) {
			base := webClaimEvidenceReviewFixture()
			return NewWebReviewClaimCoverageJob(WebReviewClaimLeafInput{
				ExactQuestion:  base.ExactQuestion,
				Context:        context,
				ParagraphText:  base.Paragraph.Text,
				AcceptedClaims: []string{},
			})
		}},
		{name: "web grounded synthesis", job: func() (PortableJob, error) {
			base := webSynthesisFixture()
			return NewWebSynthesisParagraphCoverageJob(WebSynthesisParagraphLeafInput{
				ExactQuestion:      base.ExactQuestion,
				Context:            context,
				Evidence:           base.Evidence,
				AcceptedParagraphs: []WebGroundedParagraph{},
				MaxParagraphs:      base.MaxParagraphs,
				MaxParagraphBytes:  base.MaxParagraphBytes,
			})
		}},
		{name: "web grounded synthesis correction", job: func() (PortableJob, error) {
			input := webGroundedSynthesisCorrectionFixture()
			input.Context = context
			return NewWebGroundedSynthesisCorrectionJob(input)
		}},
		{name: "database evidence gap", job: func() (PortableJob, error) {
			return NewDatabaseEvidenceGapJob(DatabaseEvidenceGapInput{
				RequirementID: "requirement-1", ExactRequirement: "Count the exact records.",
				Context:  context,
				Evidence: []GroundedEvidenceCapsule{{ID: "E1", Text: "The count is 7."}},
			})
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
			return NewDatabaseSchemaSelectionCoverageJob(DatabaseSchemaSelectionLeafInput{
				Authority: input, SelectedRelationIDs: []string{},
			})
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
