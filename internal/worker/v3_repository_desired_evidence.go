package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (session *directCodingSession) recordDesiredRepositoryGraph(
	graph repositoryfacts.DesiredArtifactGraph,
) error {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return fmt.Errorf("record desired repository graph requires one active indexed runtime")
	}
	analysis, err := exactRepositoryChangeAnalysis(
		session.repositoryIndex.Analyses, desiredGraphAnalysisID(graph, session.repositoryIndex.Analyses),
	)
	if err != nil {
		return err
	}
	if err := graph.Validate(session.repositoryIndex.Snapshot, analysis); err != nil {
		return fmt.Errorf("record desired repository graph: %w", err)
	}
	raw, err := json.Marshal(graph)
	if err != nil {
		return fmt.Errorf("encode desired repository graph: %w", err)
	}
	return session.runtime.writeEvidence(evidence.Record{
		Kind: evidence.KindRepositoryDesiredGraph, SourceType: "repository",
		SourceRef: graph.ID, Hash: strings.TrimPrefix(graph.ID, "desired_graph_"),
		Excerpt: string(raw), Confidence: 1,
		Summary: fmt.Sprintf("Code-owned desired repository graph contains %d artifact truths.", len(graph.Artifacts)),
		Metadata: map[string]any{
			"snapshot_id": graph.SnapshotID, "artifact_count": len(graph.Artifacts),
		},
	})
}

func desiredGraphAnalysisID(
	graph repositoryfacts.DesiredArtifactGraph,
	analyses []repositoryfacts.Analysis,
) string {
	for _, analysis := range analyses {
		if analysis.SnapshotID != graph.SnapshotID || analysis.Adapter.Name != "go" {
			continue
		}
		for _, desired := range graph.Artifacts {
			for _, artifact := range analysis.Artifacts {
				if artifact.ID == desired.PackageArtifactID {
					return analysis.ID
				}
			}
		}
	}
	return ""
}
