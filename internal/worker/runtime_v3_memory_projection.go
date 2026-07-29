package worker

import (
	"strings"

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
	for _, item := range retrieval.Items {
		reason, excerpt := eligibleV3MemoryReference(item, intent.MemoryMode, projectScope, sessionScope)
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

func eligibleV3MemoryReference(item artifacts.RetrievalItem, mode string, projectScope, sessionScope string) (string, string) {
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
	if item.Score < 0.35 {
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
	excerpt := compactMemoryReference(item.Content, 360)
	if excerpt == "" {
		return "empty_content", ""
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

func compactMemoryReference(content string, maxRunes int) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if clean == "" {
		return ""
	}
	runes := []rune(clean)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return clean
	}
	if maxRunes < 4 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:maxRunes-3])) + "..."
}
