package worker

import (
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func containsTag(tags []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, tag := range tags {
		if strings.ToLower(strings.TrimSpace(tag)) == target {
			return true
		}
	}
	return false
}

func mergeMemoryMatches(primary, secondary []model.MemoryMatch) []model.MemoryMatch {
	merged := make([]model.MemoryMatch, 0, len(primary)+len(secondary))
	indexByID := make(map[int64]int, len(primary)+len(secondary))
	merge := func(item model.MemoryMatch) {
		item.Tags = appendUnique(nil, item.Tags...)
		if index, exists := indexByID[item.ID]; exists {
			current := merged[index]
			current.Tags = appendUnique(current.Tags, item.Tags...)
			if item.Score > current.Score {
				current.Score = item.Score
			}
			if item.CreatedAt.After(current.CreatedAt) {
				current.CreatedAt = item.CreatedAt
			}
			if strings.TrimSpace(current.Content) == "" {
				current.Content = item.Content
			}
			merged[index] = current
			return
		}
		indexByID[item.ID] = len(merged)
		merged = append(merged, item)
	}
	for _, item := range primary {
		merge(item)
	}
	for _, item := range secondary {
		merge(item)
	}
	return merged
}

func orderV3MemoryMatches(matches []model.MemoryMatch, projectScope, sessionScope string, limit int) []model.MemoryMatch {
	ordered := mergeMemoryMatches(matches, nil)
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Score != ordered[right].Score {
			return ordered[left].Score > ordered[right].Score
		}
		leftSession := containsTag(ordered[left].Tags, sessionScope)
		rightSession := containsTag(ordered[right].Tags, sessionScope)
		if leftSession != rightSession {
			return leftSession
		}
		leftProject := containsTag(ordered[left].Tags, projectScope)
		rightProject := containsTag(ordered[right].Tags, projectScope)
		if leftProject != rightProject {
			return leftProject
		}
		if !ordered[left].CreatedAt.Equal(ordered[right].CreatedAt) {
			return ordered[left].CreatedAt.After(ordered[right].CreatedAt)
		}
		return ordered[left].ID > ordered[right].ID
	})
	if limit > 0 && len(ordered) > limit {
		ordered = ordered[:limit]
	}
	return ordered
}
