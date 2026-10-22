package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

var ErrObjectiveSessionContextCapacity = errors.New("objective session context capacity exceeded")

func requireObjectiveSessionContextAdmissionTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	kind conversationFollowupKind,
	exactText string,
) error {
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return err
	}
	if !exists || binding.Mode != model.ChannelModeAssistant {
		return nil
	}
	followups, err := loadConversationSessionFollowupsTx(
		ctx,
		tx,
		binding.ChannelID,
		job.ID,
	)
	if err != nil {
		return err
	}

	total := 0
	for _, followup := range followups {
		if len(followup.contextText) < 2 || followup.contextText[:2] != "\n\n" {
			return fmt.Errorf("job %d session context has invalid framing", job.ID)
		}
		total += len(followup.contextText)
	}
	switch kind {
	case conversationFollowupUserTurn:
		return model.ValidateChannelMessage(model.ChannelMessageRoleUser, exactText)
	case conversationFollowupRedirect, conversationFollowupInterruption, conversationFollowupCancellation:
		return nil
	default:
		return fmt.Errorf("active objective context cannot admit unregistered event kind %q", kind)
	}
}
