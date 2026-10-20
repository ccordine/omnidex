package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func (client *Client) ResearchHistory(ctx context.Context, channel model.Channel, identity string) ([]sessionintent.ResearchTurn, error) {
	return client.ResearchConversationHistory(ctx, channel, identity, "")
}

func (client *Client) ResearchConversationHistory(ctx context.Context, channel model.Channel, identity string, conversation model.LifecycleOperationID) ([]sessionintent.ResearchTurn, error) {
	history := []sessionintent.ResearchTurn{}
	for offset := 0; ; offset += sessionintent.ResearchHistoryPageSize {
		var page []sessionintent.ResearchTurn
		err := client.doJSON(ctx, http.MethodGet, "/v1/channels/"+string(channel.ID)+"/session/research?workspace_identity="+url.QueryEscape(identity)+"&conversation_id="+url.QueryEscape(string(conversation))+"&offset="+strconv.Itoa(offset), nil, &page, http.StatusOK)
		if err != nil {
			return nil, err
		}
		history = append(history, page...)
		if len(page) < sessionintent.ResearchHistoryPageSize {
			return history, nil
		}
	}
}

func (client *Client) Research(ctx context.Context, channel model.Channel, request sessionintent.ResearchRequest) (sessionintent.ResearchTurn, error) {
	var result sessionintent.ResearchTurn
	if err := ValidateSessionTurnText(request.Text); err != nil {
		return result, err
	}
	scoped := *client
	transport := *client.httpClient
	transport.Timeout = 130 * time.Second
	scoped.httpClient = &transport
	err := scoped.doJSON(ctx, http.MethodPost, "/v1/channels/"+string(channel.ID)+"/session/research", request, &result, http.StatusOK)
	if err == nil && (result.ConversationID != request.ConversationID || result.OperationID != request.OperationID || result.Question != request.Text || (result.Answer == "") == (result.Error == "")) {
		err = fmt.Errorf("research receipt differs from its question or lacks an outcome")
	}
	return result, err
}

// ResearchOutcome observes the exact operation after a canceled transport. It
// never retries the POST and therefore cannot accidentally start canceled work.
func (client *Client) ResearchOutcome(parent context.Context, channel model.Channel, request sessionintent.ResearchRequest, timeout time.Duration) (sessionintent.ResearchTurn, []sessionintent.ResearchTurn, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	delay := 100 * time.Millisecond
	for {
		history, err := client.ResearchConversationHistory(ctx, channel, request.WorkspaceIdentity, request.ConversationID)
		if err != nil {
			return sessionintent.ResearchTurn{}, nil, err
		}
		for _, turn := range history {
			if turn.OperationID == request.OperationID {
				if turn.Question != request.Text {
					return turn, nil, fmt.Errorf("Research outcome changed its question")
				}
				return turn, history, nil
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return sessionintent.ResearchTurn{}, nil, fmt.Errorf("Research cancellation outcome remains unproven: %w", ctx.Err())
		case <-timer.C:
		}
		delay = min(delay*2, time.Second)
	}
}
