package main

import (
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func parseCLIMemoryCategories(value string) ([]model.MemoryCategory, error) {
	if value == "" {
		return nil, nil
	}
	return model.ParseMemoryCategories(strings.Split(value, ","))
}

func parseCLIMemoryTags(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	tags := strings.Split(value, ",")
	if err := model.ValidateMemoryInputTags(tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func parseCLIMemoryScope(projectID int64, channelID string) (model.MemoryScope, error) {
	scope := model.MemoryScope{ProjectID: projectID, ChannelID: model.ChannelID(channelID)}
	if err := scope.Validate(); err != nil {
		return model.MemoryScope{}, err
	}
	return scope, nil
}

func combineMemoryTags(parts ...[]string) ([]string, error) {
	count := 0
	for _, values := range parts {
		count += len(values)
	}
	tags := make([]string, 0, count)
	for _, values := range parts {
		tags = append(tags, values...)
	}
	if err := model.ValidateMemoryInputTags(tags); err != nil {
		return nil, err
	}
	return tags, nil
}
