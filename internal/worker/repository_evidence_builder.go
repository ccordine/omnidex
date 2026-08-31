package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

type repositoryEvidenceBuilder interface {
	Build(context.Context, repositoryretrieval.Request) (repositoryretrieval.EvidencePack, error)
}

type existingRepositoryEvidenceBuild func(string) (repositoryretrieval.EvidencePack, error)

func newExistingRepositoryEvidenceRequest(
	projectID int64,
	analysisID string,
	codeOwnedQuery string,
) (repositoryretrieval.Request, error) {
	if projectID < 1 {
		return repositoryretrieval.Request{}, fmt.Errorf("repository evidence request requires durable project authority")
	}
	if strings.TrimSpace(analysisID) == "" {
		return repositoryretrieval.Request{}, fmt.Errorf("repository evidence request requires one analysis ID")
	}
	if _, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts, codeOwnedQuery,
	); err != nil {
		return repositoryretrieval.Request{}, fmt.Errorf("repository evidence code-owned query: %w", err)
	}
	return repositoryretrieval.Request{
		ProjectID:  projectID,
		AnalysisID: analysisID,
		Operation:  repositoryretrieval.OperationSemanticExcerpts,
		Query:      codeOwnedQuery,
		Limits: repositoryretrieval.Limits{
			MaxSymbols: 8, MaxEdges: 32, MaxSpanBytes: 4 * 1024, MaxPackBytes: 9 * 1024,
		},
	}, nil
}

func (session *directCodingSession) buildExistingRepositoryEvidence(
	codeOwnedQuery string,
) (repositoryretrieval.EvidencePack, error) {
	if session == nil || session.runtime == nil || session.runtime.svc == nil ||
		session.runtime.svc.repo == nil || session.repositoryIndex == nil {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf(
			"repository evidence acquisition requires runtime, store, and immutable index authority",
		)
	}
	if session.runtime.svc.repositoryRetrieval == nil {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("repository evidence retrieval is unavailable")
	}
	projectID, err := session.runtime.svc.repo.JobProjectID(session.runtime.ctx, session.runtime.claim.Job.ID)
	if err != nil {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("resolve repository evidence project authority: %w", err)
	}
	if projectID < 1 {
		return repositoryretrieval.EvidencePack{}, fmt.Errorf("repository evidence requires durable project authority")
	}
	return buildObjectiveRepositoryEvidence(
		session.runtime.ctx, session.runtime.svc.repositoryRetrieval,
		projectID, session.repositoryIndex.Analyses, codeOwnedQuery,
	)
}

func (session *directCodingSession) recordExistingRepositoryEvidence(
	codeOwnedQuery string,
	pack repositoryretrieval.EvidencePack,
) error {
	if err := pack.ValidateForRequest(repositoryretrieval.OperationSemanticExcerpts, codeOwnedQuery); err != nil {
		return fmt.Errorf("record repository evidence: %w", err)
	}
	if session == nil || session.runtime == nil {
		return fmt.Errorf("record repository evidence requires a runtime")
	}
	packBytes, err := evidencePackBytes(pack)
	if err != nil {
		return err
	}
	return session.runtime.writeEvidence(evidence.Record{
		Kind: evidence.KindRepositoryEvidence, SourceType: "repository", SourceRef: pack.ID,
		Summary:    fmt.Sprintf("Bounded repository evidence contains %d symbols and %d relations.", len(pack.Symbols), len(pack.Relations)),
		Confidence: 1,
		Metadata: map[string]any{
			"snapshot_id": pack.SnapshotID, "analysis_id": pack.AnalysisID,
			"operation": repositoryretrieval.OperationSemanticExcerpts, "code_owned_query": codeOwnedQuery,
			"pack_bytes": packBytes,
		},
	})
}

func evidencePackBytes(pack repositoryretrieval.EvidencePack) (int, error) {
	raw, err := json.Marshal(pack)
	if err != nil {
		return 0, fmt.Errorf("encode repository evidence pack: %w", err)
	}
	return len(raw), nil
}
