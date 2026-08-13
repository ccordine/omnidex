package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

type runtimeConversationCandidateProvider struct {
	runtime *nativeRuntimeV3
}

func (provider runtimeConversationCandidateProvider) Candidates(
	ctx context.Context,
	job model.Job,
) (conversationCandidateSet, error) {
	if provider.runtime == nil || provider.runtime.svc == nil || provider.runtime.svc.repo == nil {
		return conversationCandidateSet{}, fmt.Errorf("conversation candidate repository is unavailable")
	}
	set, err := provider.runtime.svc.repo.ConversationCandidateAuthorities(ctx, job)
	if err != nil {
		return conversationCandidateSet{}, err
	}
	return conversationCandidateSet{
		Turns: append([]assemblyline.ConversationContextTurn(nil), set.Turns...),
		AssistantResults: append(
			[]assemblyline.ConversationSelectedAssistantResult(nil), set.AssistantResults...,
		),
	}, nil
}
