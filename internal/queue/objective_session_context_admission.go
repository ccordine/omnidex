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
	if len(followups) >= MaxChannelSessionTurns {
		return fmt.Errorf(
			"%w: job %d already has the maximum %d persisted context events",
			ErrObjectiveSessionContextCapacity,
			job.ID,
			MaxChannelSessionTurns,
		)
	}
	total := 0
	for _, followup := range followups {
		if len(followup.contextText) < 2 || followup.contextText[:2] != "\n\n" {
			return fmt.Errorf("job %d session context has invalid framing", job.ID)
		}
		total += len(followup.contextText)
	}
	var prospective string
	switch kind {
	case conversationFollowupUserTurn:
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, exactText); err != nil {
			return err
		}
		prospective = "user follow-up:\n" + exactText
	case conversationFollowupRedirect:
		prospective = "user redirect:\n" + exactText
	case conversationFollowupInterruption:
		prospective = "user interruption:\n" + exactText
	case conversationFollowupCancellation:
		prospective = "user cancellation:\n" + exactText
	default:
		return fmt.Errorf("active objective context cannot admit unregistered event kind %q", kind)
	}
	projectedBytes := total + 2 + len(prospective)
	if projectedBytes > MaxObjectiveSessionContextBytes {
		return fmt.Errorf(
			"%w: job %d would require %d bytes beyond the explicit %d-byte bound",
			ErrObjectiveSessionContextCapacity,
			job.ID,
			projectedBytes,
			MaxObjectiveSessionContextBytes,
		)
	}
	return nil
}
