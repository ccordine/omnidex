package worker

import (
	"sort"
	"strings"
	"unicode"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
)

const memoryAuthorityReferenceOnly = "historical_reference_only"

type v3MemoryReference struct {
	MemoryID  int64    `json:"memory_id"`
	Kind      string   `json:"kind"`
	Excerpt   string   `json:"excerpt"`
	Tags      []string `json:"tags,omitempty"`
	Score     float64  `json:"score"`
	Authority string   `json:"authority"`
}

type v3MemoryProjection struct {
	Authority       string              `json:"authority"`
	References      []v3MemoryReference `json:"references"`
	Omitted         int                 `json:"omitted"`
	OmittedByReason map[string]int      `json:"omitted_by_reason,omitempty"`
}

func projectV3Memory(intent artifacts.IntentArtifact, retrieval artifacts.RetrievalArtifact, projectScope, sessionScope string, limit int) v3MemoryProjection {
	projection := v3MemoryProjection{
		Authority:       memoryAuthorityReferenceOnly,
		References:      []v3MemoryReference{},
		OmittedByReason: map[string]int{},
	}
	if limit <= 0 || intent.MemoryMode == artifacts.MemoryModeOff {
		projection.Omitted = len(retrieval.Items)
		if projection.Omitted > 0 {
			projection.OmittedByReason["memory_disabled"] = projection.Omitted
		}
		return projection
	}
	objectiveTokens := significantMemoryTokens(intentMemoryObjectiveText(intent))
	for _, item := range retrieval.Items {
		reason, excerpt := eligibleV3MemoryReference(item, intent.MemoryMode, objectiveTokens, projectScope, sessionScope)
		if reason != "" {
			projection.Omitted++
			projection.OmittedByReason[reason]++
			continue
		}
		projection.References = append(projection.References, v3MemoryReference{
			MemoryID:  item.ID,
			Kind:      strings.TrimSpace(item.Kind),
			Excerpt:   excerpt,
			Tags:      uniqueStrings(item.Tags),
			Score:     item.Score,
			Authority: memoryAuthorityReferenceOnly,
		})
		if len(projection.References) >= limit {
			projection.Omitted += len(retrieval.Items) - len(projection.References) - projection.Omitted
			break
		}
	}
	return projection
}

func eligibleV3MemoryReference(item artifacts.RetrievalItem, mode string, objectiveTokens map[string]struct{}, projectScope, sessionScope string) (string, string) {
	kind := strings.ToLower(strings.TrimSpace(item.Kind))
	if kind == model.MemoryKindInstruction {
		return "instruction_kind", ""
	}
	if kind == model.MemoryKindProcedural && mode != artifacts.MemoryModeExplicitRecall {
		return "procedural_not_explicitly_recalled", ""
	}
	if kind == model.MemoryKindEpisodic && mode != artifacts.MemoryModeExplicitRecall && !containsTag(item.Tags, sessionScope) {
		return "episodic_outside_session", ""
	}
	if item.Score < 0.35 && mode != artifacts.MemoryModeExplicitRecall {
		return "low_relevance_score", ""
	}
	if conflictingProjectScope(item.Tags, projectScope) {
		return "different_project", ""
	}
	trusted := containsTag(item.Tags, model.MemoryTrustTagApproved) || containsTag(item.Tags, model.MemoryTrustTagDurable)
	currentSession := sessionScope != "" && containsTag(item.Tags, sessionScope)
	if !trusted && !currentSession {
		return "untrusted", ""
	}
	excerpt := relevantMemoryExcerpt(item.Content, objectiveTokens, mode == artifacts.MemoryModeExplicitRecall)
	if excerpt == "" {
		return "no_objective_overlap", ""
	}
	return "", excerpt
}

func conflictingProjectScope(tags []string, projectScope string) bool {
	projectScope = strings.ToLower(strings.TrimSpace(projectScope))
	if projectScope == "" {
		return false
	}
	hasProjectTag := false
	for _, tag := range tags {
		clean := strings.ToLower(strings.TrimSpace(tag))
		if !strings.HasPrefix(clean, "project:") {
			continue
		}
		hasProjectTag = true
		if clean == projectScope {
			return false
		}
	}
	return hasProjectTag
}

func intentMemoryObjectiveText(intent artifacts.IntentArtifact) string {
	parts := []string{intent.UserGoal}
	for _, objective := range intent.Objectives {
		parts = append(parts, objective.Description)
		parts = append(parts, objective.AcceptanceCriteria...)
	}
	return strings.Join(parts, " ")
}

func relevantMemoryExcerpt(content string, objectiveTokens map[string]struct{}, explicitRecall bool) string {
	minimumOverlap := 2
	if explicitRecall || len(objectiveTokens) < 4 {
		minimumOverlap = 1
	}
	parts := strings.FieldsFunc(strings.TrimSpace(content), func(r rune) bool {
		return r == '\n' || r == '.' || r == '!' || r == '?'
	})
	selected := make([]string, 0, 2)
	for _, part := range parts {
		clean := strings.Join(strings.Fields(part), " ")
		if clean == "" || tokenOverlap(significantMemoryTokens(clean), objectiveTokens) < minimumOverlap {
			continue
		}
		selected = append(selected, clean)
		if len(selected) == 2 {
			break
		}
	}
	excerpt := strings.Join(selected, ". ")
	if len(excerpt) > 360 {
		excerpt = strings.TrimSpace(excerpt[:357]) + "..."
	}
	return excerpt
}

func significantMemoryTokens(value string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := map[string]struct{}{}
	for _, field := range fields {
		if tokens := memoryCJKTokens(field); len(tokens) > 0 {
			for _, token := range tokens {
				out[token] = struct{}{}
			}
			continue
		}
		if len([]rune(field)) < 3 || memoryProjectionStopword(field) {
			continue
		}
		out[field] = struct{}{}
	}
	return out
}

func memoryCJKTokens(value string) []string {
	runes := []rune(value)
	containsCJK := false
	for _, current := range runes {
		if unicode.In(current, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			containsCJK = true
			break
		}
	}
	if !containsCJK {
		return nil
	}
	out := make([]string, 0, len(runes)*2)
	for size := 2; size <= 3; size++ {
		for start := 0; start+size <= len(runes); start++ {
			valid := true
			for _, current := range runes[start : start+size] {
				if !unicode.In(current, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
					valid = false
					break
				}
			}
			if valid {
				out = append(out, string(runes[start:start+size]))
			}
		}
	}
	return out
}

func tokenOverlap(left, right map[string]struct{}) int {
	count := 0
	for token := range left {
		if _, ok := right[token]; ok {
			count++
		}
	}
	return count
}

func memoryProjectionStopword(token string) bool {
	stopwords := []string{
		"about", "after", "again", "also", "application", "before", "build", "change", "create", "current",
		"from", "have", "implement", "improve", "into", "make", "more", "project", "should", "system", "that",
		"their", "then", "there", "these", "they", "this", "through", "using", "with", "would", "your",
	}
	index := sort.SearchStrings(stopwords, token)
	return index < len(stopwords) && stopwords[index] == token
}
