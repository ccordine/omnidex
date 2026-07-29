package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
)

const maxMemoryRetrievalLimit = 64

type v3MemoryAuthorityContextKey struct{}

type v3MemoryAuthority struct {
	Intent       artifacts.IntentArtifact
	ProjectScope string
	SessionScope string
	Limit        int
}

func (s *Service) memoryAuthorityForV3Job(ctx context.Context, job model.Job) (v3MemoryAuthority, error) {
	if s == nil || s.repo == nil {
		return v3MemoryAuthority{}, fmt.Errorf("memory.retrieve requires the authoritative repository")
	}
	if s.retrievalLimit < 1 || s.retrievalLimit > maxMemoryRetrievalLimit {
		return v3MemoryAuthority{}, fmt.Errorf("memory.retrieve requires a validated retrieval limit, received %d", s.retrievalLimit)
	}
	intent, err := requireArtifactPayload[artifacts.IntentArtifact](ctx, s.repo, job.ID, artifacts.KindIntent)
	if err != nil {
		return v3MemoryAuthority{}, err
	}
	if intent.MemoryMode == artifacts.MemoryModeOff || !containsString(intent.RequiredCapabilities, capabilityMemoryRead) {
		return v3MemoryAuthority{}, fmt.Errorf("memory.retrieve is not authorized by the accepted intent for job %d", job.ID)
	}
	return v3MemoryAuthority{
		Intent:       intent,
		ProjectScope: projectTag(job),
		SessionScope: sessionTagForJob(job),
		Limit:        s.retrievalLimit,
	}, nil
}

func withV3MemoryAuthority(ctx context.Context, authority v3MemoryAuthority) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("memory tool context is required")
	}
	if strings.TrimSpace(authority.Intent.UserGoal) == "" || authority.Limit < 1 || authority.Limit > maxMemoryRetrievalLimit {
		return nil, fmt.Errorf("complete server-authoritative memory context is required")
	}
	return context.WithValue(ctx, v3MemoryAuthorityContextKey{}, authority), nil
}

func v3MemoryAuthorityFromContext(ctx context.Context) (v3MemoryAuthority, error) {
	if ctx == nil {
		return v3MemoryAuthority{}, fmt.Errorf("memory.retrieve requires a server-authoritative intent context")
	}
	authority, ok := ctx.Value(v3MemoryAuthorityContextKey{}).(v3MemoryAuthority)
	if !ok || strings.TrimSpace(authority.Intent.UserGoal) == "" || authority.Limit < 1 || authority.Limit > maxMemoryRetrievalLimit {
		return v3MemoryAuthority{}, fmt.Errorf("memory.retrieve requires a server-authoritative intent context")
	}
	return authority, nil
}

func v3ToolRequiresMemoryAuthority(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "memory.retrieve", "memory":
		return true
	default:
		return false
	}
}

func projectV3MemoryToolResult(intent artifacts.IntentArtifact, ranked []model.MemoryMatch, projectScope, sessionScope string, limit int) (artifacts.RetrievalArtifact, []model.MemoryMatch) {
	rawItems := make([]artifacts.RetrievalItem, 0, len(ranked))
	byID := make(map[int64]model.MemoryMatch, len(ranked))
	for _, match := range ranked {
		rawItems = append(rawItems, artifacts.RetrievalItem{
			ID:         match.ID,
			Kind:       match.Kind,
			Content:    match.Content,
			Tags:       append([]string(nil), match.Tags...),
			Categories: append([]string(nil), match.Categories...),
			Score:      match.Score,
		})
		byID[match.ID] = match
	}
	projection := projectV3Memory(intent, artifacts.RetrievalArtifact{Items: rawItems}, projectScope, sessionScope, limit)
	items := make([]artifacts.RetrievalItem, 0, len(projection.References))
	projectedMatches := make([]model.MemoryMatch, 0, len(projection.References))
	for _, reference := range projection.References {
		original, ok := byID[reference.MemoryID]
		if !ok {
			continue
		}
		items = append(items, artifacts.RetrievalItem{
			ID:         reference.MemoryID,
			Kind:       reference.Kind,
			Content:    reference.Excerpt,
			Tags:       append([]string(nil), reference.Tags...),
			Categories: append([]string(nil), original.Categories...),
			Score:      reference.Score,
		})
		projectedMatches = append(projectedMatches, model.MemoryMatch{
			ID:         reference.MemoryID,
			Kind:       reference.Kind,
			Content:    reference.Excerpt,
			Tags:       append([]string(nil), reference.Tags...),
			Categories: append([]string(nil), original.Categories...),
			Score:      reference.Score,
			CreatedAt:  original.CreatedAt,
		})
	}
	summary := fmt.Sprintf("historical memory projection: included=%d omitted=%d authority=%s", len(items), projection.Omitted, projection.Authority)
	return artifacts.RetrievalArtifact{
		Summary:         summary,
		Authority:       projection.Authority,
		Items:           items,
		Omitted:         projection.Omitted,
		OmittedByReason: projection.OmittedByReason,
	}, projectedMatches
}
