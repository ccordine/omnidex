package worker

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func resolveObjectiveConversationContext(
	ctx context.Context,
	job model.Job,
	authority turnAuthority,
	provider objectiveConversationCandidateProvider,
	station objectiveContextSelectionStation,
) (turnAuthority, int, error) {
	if provider == nil {
		return turnAuthority{}, 0, fmt.Errorf("conversation candidate authority provider is unavailable")
	}
	set, err := provider.Candidates(ctx, job)
	if err != nil {
		return turnAuthority{}, 0, err
	}
	set.Turns = append([]assemblyline.ConversationContextTurn(nil), set.Turns...)
	set.AssistantResults = cloneSelectedAssistantResults(set.AssistantResults)
	if len(set.Turns) == 0 {
		if len(set.AssistantResults) != 0 {
			return turnAuthority{}, 0, fmt.Errorf("conversation candidate provider returned orphaned assistant results")
		}
		return authority, 0, nil
	}
	input := assemblyline.ConversationContextSelectionInput{
		ExactInstruction:     authority.Instruction,
		MaxSelectedBytes:     assemblyline.MaxSelectedConversationProjectionBytes,
		CandidateAuthorities: set.Turns,
	}
	if _, err := assemblyline.NewConversationContextSelectionJob(input); err != nil {
		return turnAuthority{}, 0, err
	}
	if err := validateConversationCandidateResults(set); err != nil {
		return turnAuthority{}, 0, err
	}
	if station == nil {
		return turnAuthority{}, 0, fmt.Errorf("conversation context-selection station is unavailable")
	}
	decision, receipt, err := station.Select(ctx, input)
	if err != nil {
		return turnAuthority{}, 0, err
	}
	if err := ctx.Err(); err != nil {
		return turnAuthority{}, 0, err
	}
	if receipt.Calls < 1 || receipt.Calls > maxTypedWorkerAttempts {
		return turnAuthority{}, 0, fmt.Errorf(
			"conversation context-selection station reported %d calls outside the bounded correction budget",
			receipt.Calls,
		)
	}
	if err := decision.ValidateFor(input); err != nil {
		return turnAuthority{}, 0, err
	}
	selected := make(map[int64]struct{}, len(decision.ReferencedUserMessageIDs))
	for _, messageID := range decision.ReferencedUserMessageIDs {
		selected[messageID] = struct{}{}
	}
	for _, turn := range set.Turns {
		if turn.Role != assemblyline.ConversationContextUser {
			continue
		}
		if _, ok := selected[turn.MessageID]; !ok {
			continue
		}
		authority.Context.UserAuthorities = append(
			authority.Context.UserAuthorities,
			assemblyline.ConversationSelectedUserAuthority{MessageID: turn.MessageID, Content: turn.Content},
		)
		for _, result := range set.AssistantResults {
			if result.UserMessageID == turn.MessageID {
				authority.Context.AssistantResults = append(authority.Context.AssistantResults, result)
				break
			}
		}
	}
	probe := assemblyline.ConversationObjectiveKindInput{
		ExactInstruction: authority.Instruction,
		Context:          assemblyline.CloneObjectiveContext(authority.Context),
	}
	if _, err := assemblyline.NewConversationObjectiveKindJob(probe); err != nil {
		return turnAuthority{}, 0, err
	}
	return authority, receipt.Calls, nil
}

func renderSelectedConversationAuthority(authority turnAuthority) []string {
	values := make([]string, 0, len(authority.Context.UserAuthorities)+len(authority.Context.AssistantResults))
	for _, selected := range authority.Context.UserAuthorities {
		values = append(values,
			"Selected prior user authority "+strconv.FormatInt(selected.MessageID, 10)+":\n"+selected.Content,
		)
		for _, result := range authority.Context.AssistantResults {
			if result.UserMessageID != selected.MessageID {
				continue
			}
			values = append(values,
				"Paired prior assistant result "+strconv.FormatInt(result.MessageID, 10)+
					" from completed job "+strconv.FormatInt(result.JobID, 10)+":\n"+result.Content,
			)
		}
	}
	return values
}

func renderCodingObjectiveAuthority(authority turnAuthority) []string {
	values := renderSelectedConversationAuthority(authority)
	if replan := authority.Context.ReplanAuthority; replan != nil {
		values = append(values, "Exact current-generation replan feedback:\n"+replan.Feedback)
	}
	return values
}

func validateConversationCandidateResults(set conversationCandidateSet) error {
	turnsByID := make(map[int64]assemblyline.ConversationContextTurn, len(set.Turns))
	for _, turn := range set.Turns {
		turnsByID[turn.MessageID] = turn
	}
	seenUsers := make(map[int64]struct{}, len(set.AssistantResults))
	for index, result := range set.AssistantResults {
		user, userExists := turnsByID[result.UserMessageID]
		assistant, assistantExists := turnsByID[result.MessageID]
		if !userExists || user.Role != assemblyline.ConversationContextUser ||
			!assistantExists || assistant.Role != assemblyline.ConversationContextAssistant ||
			assistant.PairedUserMessageID != result.UserMessageID ||
			assistant.Content != result.Content || result.JobID < 1 || result.MessageID <= result.UserMessageID {
			return fmt.Errorf("conversation candidate assistant result %d has invalid exact binding", index)
		}
		if _, duplicate := seenUsers[result.UserMessageID]; duplicate {
			return fmt.Errorf("conversation candidate user message %d has multiple assistant results", result.UserMessageID)
		}
		seenUsers[result.UserMessageID] = struct{}{}
	}
	for _, turn := range set.Turns {
		if turn.Role == assemblyline.ConversationContextAssistant {
			found := false
			for _, result := range set.AssistantResults {
				if result.MessageID == turn.MessageID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("conversation candidate assistant message %d is not bound to a completed job", turn.MessageID)
			}
		}
	}
	return nil
}

func cloneSelectedUserAuthorities(
	values []assemblyline.ConversationSelectedUserAuthority,
) []assemblyline.ConversationSelectedUserAuthority {
	return append([]assemblyline.ConversationSelectedUserAuthority(nil), values...)
}

func cloneSelectedAssistantResults(
	values []assemblyline.ConversationSelectedAssistantResult,
) []assemblyline.ConversationSelectedAssistantResult {
	return append([]assemblyline.ConversationSelectedAssistantResult(nil), values...)
}
