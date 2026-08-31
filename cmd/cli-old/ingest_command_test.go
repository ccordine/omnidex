package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCLIIngestIsFailFastAndHasNoFilenameTags(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "postgres-react-notes.md")
	second := filepath.Join(directory, "unsupported.bin")
	if err := os.WriteFile(first, []byte("exact content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareCLIIngest(
		[]string{first, second}, 1, "channel-one", "file", "reference", "", "research", 1800, 220,
	); err == nil {
		t.Fatal("later invalid file was ignored")
	}
	inputs, _, err := prepareCLIIngest(
		[]string{first}, 1, "channel-one", "file", "reference", "", "research", 1800, 220,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range inputs[0].Tags {
		if strings.Contains(tag, "postgres") || strings.Contains(tag, "react") {
			t.Fatalf("filename token leaked into semantic tags: %#v", inputs[0].Tags)
		}
	}
}

func TestPrepareCLIIngestRequiresExactKindSourceAndCategories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("exact content"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ source, kind, categories string }{
		{"", "reference", "research"}, {" file", "reference", "research"},
		{"file", "", "research"}, {"file", "REFERENCE", "research"},
		{"file", "reference", "postgres"},
	} {
		if _, _, err := prepareCLIIngest(
			[]string{path}, 1, "channel-one", fixture.source, fixture.kind, "", fixture.categories, 1800, 220,
		); err == nil {
			t.Fatalf("invalid authority %+v was accepted", fixture)
		}
	}
}

func TestParseCLIMemoryTagsRejectsNormalizationAndAuthorityClaims(t *testing.T) {
	for _, value := range []string{
		" tag", "Tag,Tag", "tag,tag", "trust:durable", "category:database",
		"provenance:reviewed", "reviewed",
	} {
		if _, err := parseCLIMemoryTags(value); err == nil {
			t.Fatalf("inexact or reserved tag %q was accepted", value)
		}
	}
	tags, err := parseCLIMemoryTags("Tag,tag")
	if err != nil || len(tags) != 2 || tags[0] != "Tag" || tags[1] != "tag" {
		t.Fatalf("exact tags=%#v error=%v", tags, err)
	}
}
