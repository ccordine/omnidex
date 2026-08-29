package model

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxMemorySourceBytes      = 1024
	MaxMemoryContentBytes     = 1 << 20
	MaxMemoryTagBytes         = 128
	MaxMemoryTags             = 64
	MaxMemoryCategories       = 16
	MemoryEmbeddingDimensions = 768
)

type MemoryKind string
type MemorySource string
type MemoryCategory string

type MemoryScope struct {
	ProjectID int64     `json:"project_id"`
	ChannelID ChannelID `json:"channel_id"`
}

const (
	MemoryCategoryPersonal       MemoryCategory = "personal"
	MemoryCategoryProject        MemoryCategory = "project"
	MemoryCategoryLanguage       MemoryCategory = "language"
	MemoryCategoryDatabase       MemoryCategory = "database"
	MemoryCategoryInfrastructure MemoryCategory = "infrastructure"
	MemoryCategoryFrontend       MemoryCategory = "frontend"
	MemoryCategoryIntegration    MemoryCategory = "integration"
	MemoryCategoryStrategy       MemoryCategory = "strategy"
	MemoryCategoryResearch       MemoryCategory = "research"
	MemoryCategoryPreference     MemoryCategory = "preference"
	MemoryCategoryInstruction    MemoryCategory = "instruction"
	MemoryCategoryVerification   MemoryCategory = "verification"
	MemoryCategoryTroubleshoot   MemoryCategory = "troubleshooting"
	MemoryCategorySecurity       MemoryCategory = "security"
	MemoryCategoryGeneral        MemoryCategory = "general"
)

type MemoryInput struct {
	Scope      MemoryScope      `json:"scope"`
	Source     MemorySource     `json:"source"`
	Kind       MemoryKind       `json:"kind"`
	Content    string           `json:"content"`
	Tags       []string         `json:"tags"`
	Categories []MemoryCategory `json:"categories"`
}

func (scope MemoryScope) Validate() error {
	if scope.ProjectID < 1 {
		return fmt.Errorf("memory scope requires a positive project identity")
	}
	if err := scope.ChannelID.Validate(); err != nil {
		return fmt.Errorf("memory scope: %w", err)
	}
	return nil
}

func ParseMemoryKind(value string) (MemoryKind, error) {
	kind := MemoryKind(value)
	switch kind {
	case MemoryKindEpisodic, MemoryKindProcedural, MemoryKindInstruction,
		MemoryKindPreference, MemoryKindReference:
		return kind, nil
	default:
		return "", fmt.Errorf("memory kind %q is not registered exact text", value)
	}
}

func ParseMemorySource(value string) (MemorySource, error) {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > MaxMemorySourceBytes {
		return "", fmt.Errorf(
			"memory source must be exact non-empty UTF-8 provenance of at most %d bytes",
			MaxMemorySourceBytes,
		)
	}
	return MemorySource(value), nil
}

func ParseMemoryCategory(value string) (MemoryCategory, error) {
	category := MemoryCategory(value)
	switch category {
	case MemoryCategoryPersonal, MemoryCategoryProject, MemoryCategoryLanguage,
		MemoryCategoryDatabase, MemoryCategoryInfrastructure, MemoryCategoryFrontend,
		MemoryCategoryIntegration, MemoryCategoryStrategy, MemoryCategoryResearch,
		MemoryCategoryPreference, MemoryCategoryInstruction, MemoryCategoryVerification,
		MemoryCategoryTroubleshoot, MemoryCategorySecurity, MemoryCategoryGeneral:
		return category, nil
	default:
		return "", fmt.Errorf("memory category %q is not registered exact text", value)
	}
}

func ParseMemoryCategories(values []string) ([]MemoryCategory, error) {
	categories := make([]MemoryCategory, len(values))
	for index, value := range values {
		category, err := ParseMemoryCategory(value)
		if err != nil {
			return nil, err
		}
		categories[index] = category
	}
	if err := ValidateMemoryCategories(categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func ValidateMemoryCategories(categories []MemoryCategory) error {
	if len(categories) > MaxMemoryCategories {
		return fmt.Errorf("memory categories exceed the %d-item bound", MaxMemoryCategories)
	}
	seen := make(map[MemoryCategory]struct{}, len(categories))
	for _, category := range categories {
		if _, err := ParseMemoryCategory(string(category)); err != nil {
			return err
		}
		if _, exists := seen[category]; exists {
			return fmt.Errorf("memory category %q is duplicated", category)
		}
		seen[category] = struct{}{}
	}
	return nil
}

func ValidateMemoryTags(tags []string) error {
	if len(tags) > MaxMemoryTags {
		return fmt.Errorf("memory tags exceed the %d-item bound", MaxMemoryTags)
	}
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if tag == "" || tag != strings.TrimSpace(tag) || !utf8.ValidString(tag) ||
			strings.ContainsRune(tag, '\x00') || len(tag) > MaxMemoryTagBytes {
			return fmt.Errorf("memory tag must be exact non-empty UTF-8 text of at most %d bytes", MaxMemoryTagBytes)
		}
		if _, exists := seen[tag]; exists {
			return fmt.Errorf("memory tag %q is duplicated", tag)
		}
		seen[tag] = struct{}{}
	}
	return nil
}

func ValidateMemoryInputTags(tags []string) error {
	if err := ValidateMemoryTags(tags); err != nil {
		return err
	}
	for _, tag := range tags {
		if tag == "reviewed" || strings.HasPrefix(tag, "trust:") ||
			strings.HasPrefix(tag, "provenance:") || strings.HasPrefix(tag, "promotion:") ||
			strings.HasPrefix(tag, "generation:") || strings.HasPrefix(tag, "category:") {
			return fmt.Errorf("memory tag %q uses a code-owned authority namespace", tag)
		}
	}
	return nil
}

func (input MemoryInput) Validate() error {
	if err := input.Scope.Validate(); err != nil {
		return err
	}
	if _, err := ParseMemorySource(string(input.Source)); err != nil {
		return err
	}
	if _, err := ParseMemoryKind(string(input.Kind)); err != nil {
		return err
	}
	if input.Content == "" || input.Content != strings.TrimSpace(input.Content) ||
		!utf8.ValidString(input.Content) || strings.ContainsRune(input.Content, '\x00') ||
		len(input.Content) > MaxMemoryContentBytes {
		return fmt.Errorf("memory content must be exact non-empty UTF-8 text of at most %d bytes", MaxMemoryContentBytes)
	}
	if err := ValidateMemoryInputTags(input.Tags); err != nil {
		return err
	}
	return ValidateMemoryCategories(input.Categories)
}
