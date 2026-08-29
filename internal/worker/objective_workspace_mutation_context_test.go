package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestObjectiveWorkspaceMutationRejectsUnresolvedHistoricalContext(t *testing.T) {
	t.Parallel()
	for _, namespace := range []string{"conversation_exchange", "durable_memory"} {
		t.Run(namespace, func(t *testing.T) {
			t.Parallel()
			contextText := "Prior context that must not enter coding."
			authority := turnAuthority{
				JobID: 41, Instruction: "Apply that change.",
				Context: assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{{
					Sources: []assemblyline.ObjectiveContextSource{{
						Namespace: namespace, CandidateID: "CTX_1",
						ContentSHA256: assemblyline.ExactObjectiveContextSHA(contextText),
					}},
					Content:       contextText,
					ContentSHA256: assemblyline.ExactObjectiveContextSHA(contextText),
				}}},
			}
			request, err := directCodingRequestFromObjectiveAuthority(authority)
			if err == nil || !strings.Contains(err.Error(), "has no registered referent resolution") {
				t.Fatalf("request=%+v error=%v", request, err)
			}
		})
	}
}

func TestObjectiveWorkspaceMutationRoutesExactSameJobReplanWithoutCapsuleProse(t *testing.T) {
	t.Parallel()
	const (
		instruction  = "Correct the current implementation."
		feedback     = "Keep the public behavior unchanged while fixing the reported defect."
		capsuleProse = "MINIFIED_HISTORY_MUST_NOT_ENTER_CODING"
	)
	authority := turnAuthority{
		JobID: 41, Instruction: instruction,
		Context: assemblyline.ObjectiveContext{
			Capsules: []assemblyline.ObjectiveContextCapsule{{
				Sources: []assemblyline.ObjectiveContextSource{{
					Namespace: "objective_replan", CandidateID: "CTX_1",
					ContentSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
				}},
				Content:       capsuleProse,
				ContentSHA256: assemblyline.ExactObjectiveContextSHA(capsuleProse),
			}},
			ReplanAuthority: &assemblyline.ObjectiveReplanAuthority{
				JobID: 41, Generation: 2, Feedback: feedback,
				FeedbackSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
			},
		},
	}
	request, err := directCodingRequestFromObjectiveAuthority(authority)
	if err != nil {
		t.Fatal(err)
	}
	if request.Instruction != instruction || len(request.Feedback) != 1 || request.Feedback[0] != feedback {
		t.Fatalf("request=%+v", request)
	}
	modelAuthority := (&directCodingSession{request: request}).directCodingAuthority()
	if strings.Contains(modelAuthority, capsuleProse) || modelAuthority != instruction+"\n"+feedback {
		t.Fatalf("coding authority=%q", modelAuthority)
	}
}

func TestObjectiveWorkspaceMutationRejectsCrossJobReplanAuthority(t *testing.T) {
	t.Parallel()
	feedback := "Correct the reported defect."
	authority := turnAuthority{
		JobID: 41, Instruction: "Correct the current implementation.",
		Context: assemblyline.ObjectiveContext{ReplanAuthority: &assemblyline.ObjectiveReplanAuthority{
			JobID: 42, Generation: 2, Feedback: feedback,
			FeedbackSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
		}},
	}
	request, err := directCodingRequestFromObjectiveAuthority(authority)
	if err == nil || !strings.Contains(err.Error(), "belongs to job 42, expected 41") {
		t.Fatalf("request=%+v error=%v", request, err)
	}
}
