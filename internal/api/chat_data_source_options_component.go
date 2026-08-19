package api

import (
	"fmt"
	"html"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	chatDataSourceOptionsTarget = "new-channel-data-source-options"
	maxChatDataSourceNameBytes  = 256
)

func renderChatDataSourceOptionsPage(
	sources []queue.DataSourceRecord,
	nextOffset *int,
	appendOptions bool,
) (chatComponentPage, error) {
	var options strings.Builder
	if !appendOptions {
		options.WriteString(`<option value="" selected>No data</option>`)
	}
	for index, source := range sources {
		if err := model.DataSourceID(source.ID).Validate(); err != nil {
			return chatComponentPage{}, fmt.Errorf("data source option %d: %w", index, err)
		}
		if err := validateChatText(source.Name, "data source name", maxChatDataSourceNameBytes); err != nil {
			return chatComponentPage{}, fmt.Errorf("data source option %d: %w", index, err)
		}
		if source.Name != strings.TrimSpace(source.Name) {
			return chatComponentPage{}, fmt.Errorf("data source option %d: data source name must be exact", index)
		}
		options.WriteString(`<option value="` + html.EscapeString(source.ID) + `">` +
			html.EscapeString(source.Name) + `</option>`)
	}
	location := "innerHTML"
	if appendOptions {
		location = "beforeend"
	}
	bundle := renderRecyclrTemplateHTML(chatDataSourceOptionsTarget, options.String(), location)
	return chatComponentPage{
		NextOffset: nextOffset,
		HasMore:    nextOffset != nil,
		HTML:       chatComponentHTML{Bundle: bundle},
	}, nil
}
