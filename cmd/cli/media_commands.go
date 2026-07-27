package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/media"
	"github.com/gryph/omnidex/internal/model"
)

func runMediaIndex(c *client.Client, args []string) {
	fs := flag.NewFlagSet("media-index", flag.ExitOnError)
	root := fs.String("root", ".", "media library root directory")
	source := fs.String("source", "media", "memory source prefix")
	kind := fs.String("kind", model.MemoryKindReference, "memory kind")
	tags := fs.String("tags", "", "comma-separated base tags")
	episodeLimit := fs.Int("episode-limit", 0, "max episodes to index (0 = all)")
	maxLinesPerChunk := fs.Int("lines-per-chunk", 45, "subtitle lines per memory chunk")
	includeNoSubs := fs.Bool("include-no-subs", false, "store metadata chunk even when no subtitle file is found")
	dryRun := fs.Bool("dry-run", false, "scan and summarize only; do not store memory")
	_ = fs.Parse(args)

	rootPath := strings.TrimSpace(*root)
	if rootPath == "" {
		die("--root is required")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		die(err.Error())
	}

	episodes, err := media.DiscoverEpisodes(absRoot, *episodeLimit)
	if err != nil {
		die(err.Error())
	}
	if len(episodes) == 0 {
		fmt.Println("no media episodes found")
		return
	}

	baseTags := splitTags(*tags)
	totalEpisodes := 0
	episodesWithSubs := 0
	episodesWithoutSubs := 0
	totalChunks := 0
	totalStored := 0
	totalLines := 0
	errorsCount := 0

	for _, ep := range episodes {
		totalEpisodes++
		episodeTags := mergeTags(baseTags, media.EpisodeTags(ep), ingest.InferTagsFromPath(ep.VideoPath, "subtitle"))
		metaContent := buildEpisodeMetadataContent(ep)
		slug := sanitizeMemorySourceToken(fmt.Sprintf("%s-s%02de%02d", ep.ShowSlug, ep.Season, ep.Episode))
		if slug == "" {
			slug = sanitizeMemorySourceToken(strings.TrimSuffix(filepath.Base(ep.VideoPath), filepath.Ext(ep.VideoPath)))
		}
		if slug == "" {
			slug = fmt.Sprintf("episode-%d", totalEpisodes)
		}

		if ep.SubtitlePath == "" {
			episodesWithoutSubs++
			if *includeNoSubs {
				if *dryRun {
					fmt.Printf("dry-run metadata only: %s\n", ep.VideoPath)
				} else {
					sourceLabel := fmt.Sprintf("%s:%s#meta", strings.TrimSpace(*source), slug)
					if _, err := c.AddMemory(context.Background(), sourceLabel, *kind, metaContent, episodeTags); err != nil {
						errorsCount++
						fmt.Fprintf(os.Stderr, "warn: metadata store failed for %s: %v\n", ep.VideoPath, err)
					} else {
						totalStored++
					}
				}
			}
			continue
		}

		lines, err := media.ParseSubtitleLines(ep.SubtitlePath)
		if err != nil {
			errorsCount++
			fmt.Fprintf(os.Stderr, "warn: subtitle parse failed for %s: %v\n", ep.SubtitlePath, err)
			continue
		}
		if len(lines) == 0 {
			episodesWithoutSubs++
			continue
		}

		episodesWithSubs++
		totalLines += len(lines)
		chunks := media.ChunkSubtitleLines(lines, *maxLinesPerChunk)
		totalChunks += len(chunks)

		if *dryRun {
			fmt.Printf("dry-run %s lines=%d chunks=%d subtitle=%s\n", ep.VideoPath, len(lines), len(chunks), ep.SubtitlePath)
			continue
		}

		sourceMeta := fmt.Sprintf("%s:%s#meta", strings.TrimSpace(*source), slug)
		if _, err := c.AddMemory(context.Background(), sourceMeta, *kind, metaContent, episodeTags); err != nil {
			errorsCount++
			fmt.Fprintf(os.Stderr, "warn: metadata store failed for %s: %v\n", ep.VideoPath, err)
		} else {
			totalStored++
		}

		for i, chunk := range chunks {
			payload := buildSubtitleChunkContent(ep, chunk)
			sourceChunk := fmt.Sprintf("%s:%s#%03d", strings.TrimSpace(*source), slug, i+1)
			if _, err := c.AddMemory(context.Background(), sourceChunk, *kind, payload, episodeTags); err != nil {
				errorsCount++
				fmt.Fprintf(os.Stderr, "warn: chunk store failed for %s chunk=%d: %v\n", ep.SubtitlePath, i+1, err)
				continue
			}
			totalStored++
		}
	}

	fmt.Printf("media index complete root=%s episodes=%d with_subtitles=%d without_subtitles=%d subtitle_lines=%d chunks=%d stored=%d errors=%d dry_run=%v\n",
		absRoot, totalEpisodes, episodesWithSubs, episodesWithoutSubs, totalLines, totalChunks, totalStored, errorsCount, *dryRun)
}

func runMediaSearch(args []string) {
	fs := flag.NewFlagSet("media-search", flag.ExitOnError)
	root := fs.String("root", ".", "media library root directory")
	contextWindow := fs.Int("context", 2, "subtitle lines of context before/after each match")
	limit := fs.Int("limit", 20, "max matches to print")
	_ = fs.Parse(args)

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		die("media-search requires a query text")
	}

	rootPath := strings.TrimSpace(*root)
	if rootPath == "" {
		die("--root is required")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		die(err.Error())
	}

	matches, err := media.SearchSubtitleLines(absRoot, query, *contextWindow, *limit)
	if err != nil {
		die(err.Error())
	}
	if len(matches) == 0 {
		fmt.Printf("no subtitle matches for %q under %s\n", query, absRoot)
		return
	}

	for i, match := range matches {
		fmt.Printf("[%d] file=%s", i+1, match.SubtitlePath)
		if match.Show != "" {
			fmt.Printf(" show=%s", match.Show)
		}
		if match.Season > 0 && match.Episode > 0 {
			fmt.Printf(" episode=S%02dE%02d", match.Season, match.Episode)
		}
		fmt.Println("")
		for _, before := range match.Before {
			fmt.Printf("  - [%s] %s\n", safeTimestamp(before.Start), before.Text)
		}
		fmt.Printf("  > [%s] %s\n", safeTimestamp(match.Line.Start), match.Line.Text)
		for _, after := range match.After {
			fmt.Printf("  + [%s] %s\n", safeTimestamp(after.Start), after.Text)
		}
	}
}

func buildEpisodeMetadataContent(ep media.Episode) string {
	lines := []string{
		"Episode metadata",
		"video=" + ep.VideoPath,
		"subtitle=" + safeValue(ep.SubtitlePath, "(none)"),
		"show=" + safeValue(ep.Show, "(unknown)"),
		"show_slug=" + safeValue(ep.ShowSlug, "(unknown)"),
	}
	if ep.Season > 0 {
		lines = append(lines, fmt.Sprintf("season=%d", ep.Season))
	}
	if ep.Episode > 0 {
		lines = append(lines, fmt.Sprintf("episode=%d", ep.Episode))
	}
	return strings.Join(lines, "\n")
}

func buildSubtitleChunkContent(ep media.Episode, chunk media.SubtitleChunk) string {
	lines := []string{
		"Episode subtitle chunk",
		"video=" + ep.VideoPath,
		"subtitle=" + ep.SubtitlePath,
		"show=" + safeValue(ep.Show, "(unknown)"),
		"show_slug=" + safeValue(ep.ShowSlug, "(unknown)"),
	}
	if ep.Season > 0 {
		lines = append(lines, fmt.Sprintf("season=%d", ep.Season))
	}
	if ep.Episode > 0 {
		lines = append(lines, fmt.Sprintf("episode=%d", ep.Episode))
	}
	lines = append(lines,
		fmt.Sprintf("line_range=%d-%d", chunk.FromLine, chunk.ToLine),
		"start="+safeTimestamp(chunk.Start),
		"end="+safeTimestamp(chunk.End),
		"content:",
		chunk.Text,
	)
	return strings.Join(lines, "\n")
}

func sanitizeMemorySourceToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	re := regexp.MustCompile(`[^a-z0-9]+`)
	value = re.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) > 80 {
		value = strings.Trim(value[:80], "-")
	}
	return value
}

func safeTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
