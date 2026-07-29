package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"sort"
	"strings"
	"time"
)

func (r *nativeRuntimeV3) collectSubtaskResults() ([]artifacts.SubtaskResultArtifact, error) {
	values := map[string]string{}
	for _, ctxItem := range r.claim.Contexts {
		key := strings.ToLower(strings.TrimSpace(ctxItem.Key))
		if strings.HasPrefix(key, "subtask:") {
			values[key] = strings.TrimSpace(ctxItem.Value)
		}
	}
	for key, value := range r.contexts {
		key = strings.ToLower(strings.TrimSpace(key))
		if strings.HasPrefix(key, "subtask:") {
			values[key] = strings.TrimSpace(value)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	results := make([]artifacts.SubtaskResultArtifact, 0, len(keys))
	seenIDs := map[string]struct{}{}
	for _, key := range keys {
		decoder := json.NewDecoder(strings.NewReader(values[key]))
		decoder.DisallowUnknownFields()
		var result artifacts.SubtaskResultArtifact
		if err := decoder.Decode(&result); err != nil {
			return nil, fmt.Errorf("decode delegated result %s: %w", key, err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return nil, fmt.Errorf("decode delegated result %s: %w", key, err)
		}
		if err := validateV3SubtaskResult(result); err != nil {
			return nil, err
		}
		if _, duplicate := seenIDs[result.SubtaskID]; duplicate {
			return nil, fmt.Errorf("delegated result %q is duplicated", result.SubtaskID)
		}
		seenIDs[result.SubtaskID] = struct{}{}
		results = append(results, result)
	}
	return results, nil
}

func requireArtifactPayload[T any](ctx context.Context, repo *queue.Repository, jobID int64, kind string) (T, error) {
	var zero T
	if repo == nil {
		return zero, fmt.Errorf("read %s artifact: repository is required", strings.TrimSpace(kind))
	}
	if jobID <= 0 {
		return zero, fmt.Errorf("read %s artifact: valid job id is required", strings.TrimSpace(kind))
	}
	if strings.TrimSpace(kind) == "" {
		return zero, fmt.Errorf("read artifact: kind is required")
	}
	env, ok, err := repo.LatestArtifact(ctx, jobID, kind)
	if err != nil {
		return zero, fmt.Errorf("read %s artifact for job %d: %w", kind, jobID, err)
	}
	if !ok {
		return zero, fmt.Errorf("required %s artifact is missing for job %d", kind, jobID)
	}
	if err := env.Validate(); err != nil {
		return zero, fmt.Errorf("validate required %s artifact for job %d: %w", kind, jobID, err)
	}
	if env.Kind != kind {
		return zero, fmt.Errorf("required artifact kind mismatch for job %d: expected %s, received %s", jobID, kind, env.Kind)
	}
	if env.Version != "1" {
		return zero, fmt.Errorf("required %s artifact for job %d has unsupported version %q", kind, jobID, env.Version)
	}
	decoder := json.NewDecoder(strings.NewReader(string(env.Payload)))
	decoder.DisallowUnknownFields()
	var payload T
	if err := decoder.Decode(&payload); err != nil {
		return zero, fmt.Errorf("decode required %s artifact for job %d: %w", kind, jobID, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return zero, fmt.Errorf("decode required %s artifact for job %d: %w", kind, jobID, err)
	}
	return payload, nil
}

func filterDelegatedSubtasks(subtasks []artifacts.Subtask) []artifacts.Subtask {
	out := make([]artifacts.Subtask, 0, len(subtasks))
	seen := map[string]struct{}{}
	for _, subtask := range subtasks {
		kind := strings.ToLower(strings.TrimSpace(subtask.Kind))
		if kind == "respond" || kind == "verify" || kind == "memory_review" {
			continue
		}
		key := kind + "::" + strings.ToLower(strings.TrimSpace(subtask.ObjectiveID)) + "::" + strings.ToLower(strings.TrimSpace(subtask.RoleID))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, subtask)
		if len(out) >= maxDelegatedSubtasks {
			break
		}
	}
	return out
}

func reviewMemoryCandidate(candidate model.MemoryCandidate) string {
	content := strings.TrimSpace(candidate.Content)
	if len(content) < 18 {
		return model.MemoryCandidateStatusRejected
	}
	groundedInInstruction := candidateProvenanceBool(candidate, "grounded_in_instruction")
	switch candidate.CandidateKind {
	case model.MemoryKindInstruction, model.MemoryKindProcedural:
		return model.MemoryCandidateStatusRejected
	case model.MemoryKindPreference:
		if groundedInInstruction || candidate.Confidence >= 0.9 {
			return model.MemoryCandidateStatusApproved
		}
	case model.MemoryKindReference:
		if candidate.Confidence >= 0.9 {
			return model.MemoryCandidateStatusApproved
		}
	}
	return model.MemoryCandidateStatusRejected
}

func candidateProvenanceBool(candidate model.MemoryCandidate, key string) bool {
	if len(candidate.Provenance) == 0 || !json.Valid(candidate.Provenance) {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(candidate.Provenance, &payload); err != nil {
		return false
	}
	raw, ok := payload[strings.TrimSpace(key)]
	if !ok {
		return false
	}
	value, ok := raw.(bool)
	return ok && value
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
