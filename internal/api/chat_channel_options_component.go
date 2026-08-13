package api

import (
	"fmt"
	"html"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	chatChannelOptionsTarget    = "channel-options"
	chatChannelPaginationTarget = "channel-options-pagination"
)

type chatChannelOptionsPage struct {
	chatComponentPage
	DefaultChannelID *model.ChannelID `json:"default_channel_id,omitempty"`
}

func renderChatChannelOptionsPage(
	channels []model.Channel,
	nextOffset *int,
	appendOptions bool,
) (chatChannelOptionsPage, error) {
	var options strings.Builder
	var defaultID *model.ChannelID
	for index, channel := range channels {
		if err := channel.ValidateStored(); err != nil {
			return chatChannelOptionsPage{}, fmt.Errorf("channel option %d: %w", index, err)
		}
		if index == 0 && !appendOptions {
			value := channel.ID
			defaultID = &value
		}
		options.WriteString(`<option value="` + html.EscapeString(string(channel.ID)) + `">` +
			html.EscapeString(channel.Name) + `</option>`)
	}
	if len(channels) == 0 && !appendOptions {
		options.WriteString(`<option value="" disabled selected>Create a workspace-bound channel</option>`)
	}
	location := "innerHTML"
	if appendOptions {
		location = "beforeend"
	}
	bundle := renderRecyclrTemplateHTML(chatChannelOptionsTarget, options.String(), location) +
		renderRecyclrTemplateHTML(chatChannelPaginationTarget, chatPaginationButton(
			"loadMoreChannels", chatChannelOptionsTarget, "channels", nextOffset, "Load more channels",
		), "innerHTML")
	return chatChannelOptionsPage{
		chatComponentPage: chatComponentPage{
			NextOffset: nextOffset, HasMore: nextOffset != nil, HTML: chatComponentHTML{Bundle: bundle},
		},
		DefaultChannelID: defaultID,
	}, nil
}
