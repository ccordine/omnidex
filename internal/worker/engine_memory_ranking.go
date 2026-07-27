package worker

import (
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"strings"
	"time"
)

func memoryTagAlignmentScore(matchTags []string, scopeTags []string) float64 {
	normalizedScope := appendUnique(nil, scopeTags...)
	normalizedMatch := appendUnique(nil, matchTags...)
	if len(normalizedScope) == 0 || len(normalizedMatch) == 0 {
		return 0
	}

	direct := 0
	relative := 0
	for _, scope := range normalizedScope {
		hasDirect := false
		hasRelative := false
		for _, tag := range normalizedMatch {
			if tag == scope {
				hasDirect = true
				break
			}
			if tagsAreRelated(tag, scope) {
				hasRelative = true
			}
		}
		if hasDirect {
			direct++
			continue
		}
		if hasRelative {
			relative++
		}
	}

	score := float64(direct) / float64(len(normalizedScope))
	score += (float64(relative) / float64(len(normalizedScope))) * 0.45
	return clamp01(score)
}

func memoryRecencyScore(createdAt time.Time, now time.Time) float64 {
	if createdAt.IsZero() {
		return 0
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if createdAt.After(now) {
		return 1
	}

	ageHours := now.Sub(createdAt).Hours()
	if ageHours <= 0 {
		return 1
	}
	score := 1.0 / (1.0 + (ageHours / 24.0))
	return clamp01(score)
}

func memoryActivityScore(match model.MemoryMatch, projectScope, sessionScope string, now time.Time) float64 {
	score := 0.0
	if projectScope != "" && containsTag(match.Tags, projectScope) {
		score += 0.35
	}
	if sessionScope != "" && containsTag(match.Tags, sessionScope) {
		score += 0.50
	}
	if strings.EqualFold(strings.TrimSpace(match.Kind), model.MemoryKindEpisodic) {
		score += 0.10
	}
	if !match.CreatedAt.IsZero() {
		if now.IsZero() {
			now = time.Now().UTC()
		}
		age := now.Sub(match.CreatedAt)
		switch {
		case age <= 15*time.Minute:
			score += 0.35
		case age <= 2*time.Hour:
			score += 0.25
		case age <= 24*time.Hour:
			score += 0.12
		}
	}
	return clamp01(score)
}

func sameTagSet(left []string, right []string) bool {
	a := appendUnique(nil, left...)
	b := appendUnique(nil, right...)
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, value := range a {
		seen[value] = struct{}{}
	}
	for _, value := range b {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}

func mergeMemoryMatches(primary []model.MemoryMatch, secondary []model.MemoryMatch) []model.MemoryMatch {
	if len(primary) == 0 && len(secondary) == 0 {
		return nil
	}

	out := make([]model.MemoryMatch, 0, len(primary)+len(secondary))
	index := map[int64]int{}
	mergeOne := func(item model.MemoryMatch) {
		item.Tags = appendUnique(nil, item.Tags...)
		if idx, ok := index[item.ID]; ok {
			existing := out[idx]
			existing.Tags = appendUnique(existing.Tags, item.Tags...)
			if item.Score > existing.Score {
				existing.Score = item.Score
			}
			if item.CreatedAt.After(existing.CreatedAt) {
				existing.CreatedAt = item.CreatedAt
			}
			if strings.TrimSpace(existing.Content) == "" {
				existing.Content = item.Content
			}
			out[idx] = existing
			return
		}

		index[item.ID] = len(out)
		out = append(out, item)
	}

	for _, item := range primary {
		mergeOne(item)
	}
	for _, item := range secondary {
		mergeOne(item)
	}
	return out
}

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

func tagsAreRelated(left string, right string) bool {
	a := strings.ToLower(strings.TrimSpace(left))
	b := strings.ToLower(strings.TrimSpace(right))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if tagFamily(a) == tagFamily(b) && strings.Contains(a, ":") && strings.Contains(b, ":") {
		return true
	}
	if len(a) >= 4 && strings.Contains(b, a) {
		return true
	}
	if len(b) >= 4 && strings.Contains(a, b) {
		return true
	}

	aTokens := tagTokenSet(a)
	bTokens := tagTokenSet(b)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return false
	}
	shared := 0
	for token := range aTokens {
		if _, ok := bTokens[token]; ok {
			shared++
		}
	}
	return shared > 0
}

func tagFamily(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return ""
	}
	if idx := strings.Index(tag, ":"); idx > 0 {
		return tag[:idx]
	}
	return tag
}

func tagTokenSet(value string) map[string]struct{} {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ':', '/', '-', '_', '.', '|', ',':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return nil
	}

	out := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		token := strings.ToLower(strings.TrimSpace(part))
		if len(token) < 2 {
			continue
		}
		out[token] = struct{}{}
	}
	return out
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func buildRetrievalContext(matches []model.MemoryMatch, budget int) string {
	if len(matches) == 0 {
		return ""
	}

	var b strings.Builder
	for i, match := range matches {
		created := "unknown"
		if !match.CreatedAt.IsZero() {
			created = match.CreatedAt.UTC().Format(time.RFC3339)
		}
		tags := strings.Join(match.Tags, "|")
		if strings.TrimSpace(tags) == "" {
			tags = "none"
		}
		categories := strings.Join(match.Categories, "|")
		if strings.TrimSpace(categories) == "" {
			categories = "none"
		}
		segment := fmt.Sprintf(
			"[%d] kind=%s score=%.4f created_at=%s categories=%s tags=%s\n%s\n\n",
			i+1,
			match.Kind,
			match.Score,
			created,
			categories,
			tags,
			strings.TrimSpace(match.Content),
		)
		if budget > 0 && b.Len()+len(segment) > budget {
			break
		}
		b.WriteString(segment)
	}
	return strings.TrimSpace(b.String())
}

func trimForBudget(value string, budget int) string {
	value = strings.TrimSpace(value)
	if budget <= 0 || len(value) <= budget {
		return value
	}
	if budget < 20 {
		return value[:budget]
	}
	return value[:budget-15] + "\n...[truncated]"
}
