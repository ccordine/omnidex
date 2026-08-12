package omni

import (
	"sort"
	"strings"
	"time"
)

type MemoryRecord struct {
	ID            int64
	AgentID       string
	Source        string
	Kind          string
	Content       string
	Tags          []string
	Priority      int
	SupersededAt  time.Time
	SupersededBy  int64
	StalenessNote string
	CreatedAt     time.Time
}

func cleanMemoryTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		clean := strings.ToLower(strings.TrimSpace(tag))
		clean = strings.ReplaceAll(clean, " ", "-")
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}
