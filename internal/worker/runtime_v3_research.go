package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

func (r *nativeRuntimeV3) runWorkspaceResearch() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	requiresWorkspace := containsString(intent.RequiredCapabilities, capabilityWorkspaceRead)
	if r.svc.workspace == nil || !r.svc.workspace.Enabled() {
		if requiresWorkspace {
			return fmt.Errorf("authoritative intent requires workspace research but workspace.research is unavailable")
		}
		artifact := artifacts.WorkspaceArtifact{Summary: "workspace capability unavailable and not required by the accepted intent"}
		if err := r.writeArtifact(artifacts.KindWorkspace, artifact); err != nil {
			return err
		}
		return r.complete("workspace", artifact.Summary, artifact.Summary)
	}
	if !requiresWorkspace {
		artifact := artifacts.WorkspaceArtifact{Summary: "workspace research not required by the authoritative intent"}
		if err := r.writeArtifact(artifacts.KindWorkspace, artifact); err != nil {
			return err
		}
		return r.complete("workspace", artifact.Summary, artifact.Summary)
	}
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "workspace_researcher", toolruntime.Call{
		Name:  "workspace.research",
		Input: map[string]any{"query": intentObjectiveSummary(intent)},
	})
	if err != nil {
		return err
	}
	artifact, err := decodeToolOutput[artifacts.WorkspaceArtifact](result)
	if err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindWorkspace, artifact); err != nil {
		return err
	}
	return r.complete("workspace", artifact.Summary, artifact.Summary)
}

func (r *nativeRuntimeV3) runMemoryRetrieval() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	if intent.MemoryMode == artifacts.MemoryModeOff {
		output := "Historical memory disabled by the authoritative intent contract."
		artifact := artifacts.RetrievalArtifact{Summary: output}
		if err := r.writeArtifact(artifacts.KindRetrieval, artifact); err != nil {
			return err
		}
		return r.complete("retrieval", output, output)
	}
	if !containsString(intent.RequiredCapabilities, capabilityMemoryRead) {
		output := "Historical memory was not requested by the authoritative intent."
		artifact := artifacts.RetrievalArtifact{Summary: output}
		if err := r.writeArtifact(artifacts.KindRetrieval, artifact); err != nil {
			return err
		}
		return r.complete("retrieval", output, output)
	}
	limit := r.svc.retrievalLimit
	scopeTags := memoryScopeTags(r.claim.Job, splitCSVTags(r.contexts["tags"]))
	sessionTag := sessionTagForJob(r.claim.Job)
	projectScope := projectTag(r.claim.Job)
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "memory_retriever", toolruntime.Call{
		Name: "memory.retrieve",
		Input: map[string]any{
			"query":      intentObjectiveSummary(intent),
			"limit":      limit,
			"scope_tags": scopeTags,
		},
	})
	if err != nil {
		return err
	}
	artifact, err := decodeToolOutput[artifacts.RetrievalArtifact](result)
	if err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindRetrieval, artifact); err != nil {
		return err
	}
	projection := projectV3Memory(intent, artifact, projectScope, sessionTag, limit)
	projection.Omitted += artifact.Omitted
	for reason, count := range artifact.OmittedByReason {
		projection.OmittedByReason[reason] += count
	}
	projectionJSON, err := json.Marshal(projection)
	if err != nil {
		return fmt.Errorf("marshal memory projection: %w", err)
	}
	r.svc.emitStepEvent(r.claim.Step.ID, "memory_projection_ready", fmt.Sprintf("included=%d omitted=%d authority=%s", len(projection.References), projection.Omitted, projection.Authority))
	output := fmt.Sprintf("memory projection: included=%d omitted=%d", len(projection.References), projection.Omitted)
	return r.complete("retrieval", output, string(projectionJSON))
}

func (r *nativeRuntimeV3) runExternalResearch() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	query := externalResearchQuery(r.claim.Job, intent)
	if !containsString(intent.RequiredCapabilities, capabilityWebSearch) {
		artifact := artifacts.WebEvidenceArtifact{Query: query, Summary: "external research not required by the authoritative intent"}
		if err := r.writeArtifact(artifacts.KindWebEvidence, artifact); err != nil {
			return err
		}
		return r.complete("web_search", artifact.Summary, artifact.Summary)
	}
	if r.svc.webSearch == nil {
		return fmt.Errorf("authoritative intent requires external research but web.search is unavailable")
	}
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "web_researcher", toolruntime.Call{
		Name:  "web.search",
		Input: map[string]any{"query": query},
	})
	if err != nil {
		return err
	}
	artifact, err := decodeToolOutput[artifacts.WebEvidenceArtifact](result)
	if err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindWebEvidence, artifact); err != nil {
		return err
	}
	return r.complete("web_search", artifact.Summary, artifact.Summary)
}

func externalResearchQuery(job model.Job, intent artifacts.IntentArtifact) string {
	if query := strings.TrimSpace(metadataString(job.Metadata, "search_query")); query != "" {
		return query
	}
	return intentObjectiveSummary(intent)
}
