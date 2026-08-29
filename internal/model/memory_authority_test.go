package model

import (
	"strings"
	"testing"
)

func TestMemoryKindRequiresRegisteredExactText(t *testing.T) {
	for _, value := range []string{
		MemoryKindEpisodic, MemoryKindProcedural, MemoryKindInstruction,
		MemoryKindPreference, MemoryKindReference,
	} {
		kind, err := ParseMemoryKind(value)
		if err != nil || string(kind) != value {
			t.Fatalf("kind %q parsed as %q error=%v", value, kind, err)
		}
	}
	for _, value := range []string{"", " reference", "REFERENCE", "unknown"} {
		if _, err := ParseMemoryKind(value); err == nil {
			t.Fatalf("noncanonical memory kind %q was accepted", value)
		}
	}
}

func TestMemorySourceIsBoundedOpaqueExactText(t *testing.T) {
	for _, value := range []string{"manual", "document:opaque-id:1", "job:7:generation:2"} {
		source, err := ParseMemorySource(value)
		if err != nil || string(source) != value {
			t.Fatalf("source %q parsed as %q error=%v", value, source, err)
		}
	}
	for _, value := range []string{"", " ", " manual", "manual ", "bad\x00source", strings.Repeat("a", MaxMemorySourceBytes+1)} {
		if _, err := ParseMemorySource(value); err == nil {
			t.Fatalf("invalid memory source %q was accepted", value)
		}
	}
}

func TestMemoryScopeRequiresExactProjectAndChannelAuthority(t *testing.T) {
	for _, scope := range []MemoryScope{
		{}, {ProjectID: 1}, {ProjectID: -1, ChannelID: "channel-one"},
		{ProjectID: 1, ChannelID: "Channel-One"},
	} {
		if err := scope.Validate(); err == nil {
			t.Fatalf("invalid memory scope %+v was accepted", scope)
		}
	}
	if err := (MemoryScope{ProjectID: 1, ChannelID: "channel-one"}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryCategoriesRejectAliasesAndUnknownValues(t *testing.T) {
	accepted := []MemoryCategory{
		MemoryCategoryProject, MemoryCategoryLanguage, MemoryCategoryDatabase,
		MemoryCategoryInfrastructure, MemoryCategoryFrontend, MemoryCategoryIntegration,
	}
	if err := ValidateMemoryCategories(accepted); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", " project", "PROJECT", "repo", "pgsql", "postgres", "react", "vite", "docker", "api", "reference", "custom-skill"} {
		if _, err := ParseMemoryCategory(value); err == nil {
			t.Fatalf("memory category alias/unknown %q was accepted", value)
		}
	}
}

func TestMemoryInputRejectsInexactTagsInsteadOfCleaningThem(t *testing.T) {
	base := MemoryInput{
		Scope:  MemoryScope{ProjectID: 1, ChannelID: "channel-one"},
		Source: MemorySource("manual"), Kind: MemoryKind(MemoryKindReference),
		Content: "exact content", Tags: []string{"scope:user"},
		Categories: []MemoryCategory{MemoryCategoryResearch},
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tags := range [][]string{
		{""}, {" tag"}, {"tag", "tag"}, {"trust:durable"},
		{"provenance:reviewed"}, {"promotion:global"}, {"generation:7"},
		{"category:database"}, {"reviewed"},
	} {
		candidate := base
		candidate.Tags = tags
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid exact tags %#v were accepted", tags)
		}
	}
}
