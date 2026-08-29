package queue

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func enqueueNarratorRoleplayTurn(
	ctx context.Context,
	repository *Repository,
	channelID model.ChannelID,
	instruction string,
) (model.ChannelMessage, model.Job, error) {
	contribution := roleplay.UserContributionDirection
	request := roleplay.UserTurnRequest{
		PersonaKind: roleplay.UserPersonaNarrator, ContributionKind: contribution,
	}
	if strings.HasPrefix(instruction, "/") {
		request.ContributionKind = roleplay.UserContributionCommand
	} else {
		request.Parts = []roleplay.UserTurnPart{{
			Kind: roleplay.UserTurnPartMessage, Text: instruction,
		}}
		instruction = "[Message]\n" + instruction
	}
	return repository.EnqueueRoleplayChannelTurn(ctx, channelID, instruction, request)
}
