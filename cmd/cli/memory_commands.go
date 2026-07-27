package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func runRemember(c *client.Client, args []string) {
	fs := flag.NewFlagSet("remember", flag.ExitOnError)
	source := fs.String("source", "manual", "memory source label")
	kind := fs.String("kind", model.MemoryKindEpisodic, "memory kind: episodic|procedural|instruction|preference|reference")
	tags := fs.String("tags", "", "comma-separated tags")
	_ = fs.Parse(args)

	content := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if content == "" {
		die("remember requires content")
	}

	tagList := splitTags(*tags)
	chunk, err := c.AddMemory(context.Background(), *source, *kind, content, tagList)
	if err != nil {
		die(err.Error())
	}

	fmt.Printf("stored memory #%d source=%s kind=%s\n", chunk.ID, chunk.Source, chunk.Kind)
}

func runMemoryCandidates(c *client.Client, args []string) {
	if len(args) == 0 {
		die("memory-candidates requires a subcommand: list|promote|reject")
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		fs := flag.NewFlagSet("memory-candidates list", flag.ExitOnError)
		jobID := fs.Int64("job-id", 0, "optional job id filter")
		status := fs.String("status", "", "optional candidate status filter")
		limit := fs.Int("limit", 50, "max candidates to return")
		_ = fs.Parse(args[1:])

		items, err := c.ListMemoryCandidates(context.Background(), *jobID, *status, *limit)
		if err != nil {
			die(err.Error())
		}
		payload, err := json.MarshalIndent(map[string]any{"memory_candidates": items}, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
	case "promote":
		fs := flag.NewFlagSet("memory-candidates promote", flag.ExitOnError)
		tier := fs.String("tier", model.MemoryCandidateStatusApproved, "target tier: approved|durable")
		_ = fs.Parse(args[1:])
		if len(fs.Args()) < 1 {
			die("memory-candidates promote requires a candidate id")
		}
		id, err := strconv.ParseInt(fs.Args()[0], 10, 64)
		if err != nil || id <= 0 {
			die("invalid candidate id")
		}
		result, err := c.PromoteMemoryCandidate(context.Background(), id, *tier)
		if err != nil {
			die(err.Error())
		}
		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
	case "reject":
		if len(args) < 2 {
			die("memory-candidates reject requires a candidate id")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || id <= 0 {
			die("invalid candidate id")
		}
		item, err := c.RejectMemoryCandidate(context.Background(), id)
		if err != nil {
			die(err.Error())
		}
		payload, err := json.MarshalIndent(map[string]any{"memory_candidate": item}, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
	default:
		die("memory-candidates requires a subcommand: list|promote|reject")
	}
}
