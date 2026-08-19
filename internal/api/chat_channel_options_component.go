package api

import (
	"fmt"
	"html"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	chatChannelOptionsTarget       = "channel-options"
	chatNewConversationOptionValue = "__omnidex_new_conversation__"
)

type chatChannelOptionsPage struct {
	chatComponentPage
}

func renderChatChannelOptionsPage(
	channels []model.Channel,
	nextOffset *int,
	appendOptions bool,
) (chatChannelOptionsPage, error) {
	var options strings.Builder
	if !appendOptions {
		options.WriteString(`<option value="" disabled selected>Choose a conversation</option>`)
		options.WriteString(`<option value="` + chatNewConversationOptionValue + `">+ New conversation…</option>`)
	}
	for index, channel := range channels {
		if err := channel.ValidateStored(); err != nil {
			return chatChannelOptionsPage{}, fmt.Errorf("channel option %d: %w", index, err)
		}
		attributes := ` data-channel-mode="` + html.EscapeString(string(channel.Mode)) + `"`
		label := channel.Name + " · " + string(channel.Mode)
		if channel.DataSourceID != "" {
			dataSourceID := html.EscapeString(string(channel.DataSourceID))
			attributes += ` data-data-source-id="` + dataSourceID + `"`
			label += " · data connected"
		}
		if channel.RoleplayViewpointCharacterID != "" {
			viewpointID := html.EscapeString(string(channel.RoleplayViewpointCharacterID))
			attributes += ` data-roleplay-viewpoint-character-id="` + viewpointID + `"`
			label += " viewpoint " + string(channel.RoleplayViewpointCharacterID)
		}
		options.WriteString(`<option value="` + html.EscapeString(string(channel.ID)) + `"` +
			attributes + `>` + html.EscapeString(label) + `</option>`)
	}
	location := "innerHTML"
	if appendOptions {
		location = "beforeend"
	}
	bundle := renderRecyclrTemplateHTML(chatChannelOptionsTarget, options.String(), location)
	return chatChannelOptionsPage{
		chatComponentPage: chatComponentPage{
			NextOffset: nextOffset, HasMore: nextOffset != nil, HTML: chatComponentHTML{Bundle: bundle},
		},
	}, nil
}
