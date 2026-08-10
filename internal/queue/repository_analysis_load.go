package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (r *Repository) RepositoryAnalysis(
	ctx context.Context,
	projectID int64,
	analysisID string,
) (repositoryfacts.Analysis, error) {
	if ctx == nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("load repository analysis requires a context")
	}
	if projectID < 1 {
		return repositoryfacts.Analysis{}, fmt.Errorf("load repository analysis requires a positive project ID")
	}
	analysisID = strings.TrimSpace(analysisID)
	if analysisID == "" {
		return repositoryfacts.Analysis{}, fmt.Errorf("load repository analysis requires an ID")
	}
	if r == nil || r.pool == nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("load repository analysis requires PostgreSQL")
	}
	analysis, _, err := loadRepositoryAnalysis(ctx, r.pool, projectID, analysisID)
	return analysis, err
}

func loadRepositoryAnalysis(
	ctx context.Context,
	query repositorySnapshotQuerier,
	projectID int64,
	analysisID string,
) (repositoryfacts.Analysis, repositoryfacts.Snapshot, error) {
	analysis := repositoryfacts.Analysis{
		Symbols:     make([]repositoryfacts.Symbol, 0),
		Artifacts:   make([]repositoryfacts.Artifact, 0),
		Edges:       make([]repositoryfacts.Edge, 0),
		Diagnostics: make([]repositoryfacts.AnalysisDiagnostic, 0),
	}
	err := query.QueryRow(ctx, `
		SELECT a.schema_version, a.id, a.snapshot_id, a.adapter_name,
		       a.adapter_version, a.complete, a.generated_at
		FROM repository_analyses a
		JOIN repository_snapshots s ON s.id=a.snapshot_id
		WHERE s.project_id=$1 AND a.id=$2
	`, projectID, analysisID).Scan(
		&analysis.Schema, &analysis.ID, &analysis.SnapshotID, &analysis.Adapter.Name,
		&analysis.Adapter.Version, &analysis.Complete, &analysis.GeneratedAt,
	)
	if err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{},
			fmt.Errorf("load repository analysis %q: %w", analysisID, err)
	}
	analysis.GeneratedAt = analysis.GeneratedAt.UTC()
	snapshot, err := loadRepositorySnapshot(ctx, query, projectID, analysis.SnapshotID)
	if err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, err
	}
	if err := loadRepositorySymbols(ctx, query, &analysis); err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, err
	}
	if err := loadRepositoryArtifacts(ctx, query, &analysis); err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, err
	}
	if err := loadRepositoryEdges(ctx, query, &analysis); err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, err
	}
	if err := loadRepositoryDiagnostics(ctx, query, &analysis); err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{}, err
	}
	if err := analysis.Validate(snapshot); err != nil {
		return repositoryfacts.Analysis{}, repositoryfacts.Snapshot{},
			fmt.Errorf("stored repository analysis %q is invalid: %w", analysisID, err)
	}
	return analysis, snapshot, nil
}

func loadRepositorySymbols(ctx context.Context, query repositorySnapshotQuerier, analysis *repositoryfacts.Analysis) error {
	rows, err := query.Query(ctx, `
		SELECT symbol_id, file_id, kind, name, qualified_name, signature,
		       start_byte, end_byte, source_sha256, origin, confidence
		FROM repository_symbols
		WHERE analysis_id=$1
		ORDER BY symbol_id ASC
	`, analysis.ID)
	if err != nil {
		return fmt.Errorf("load repository analysis symbols: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var symbol repositoryfacts.Symbol
		if err := rows.Scan(
			&symbol.ID, &symbol.FileID, &symbol.Kind, &symbol.Name, &symbol.QualifiedName,
			&symbol.Signature, &symbol.StartByte, &symbol.EndByte, &symbol.SourceSHA256,
			&symbol.Origin, &symbol.Confidence,
		); err != nil {
			return fmt.Errorf("scan repository analysis symbol: %w", err)
		}
		symbol.Adapter = analysis.Adapter
		analysis.Symbols = append(analysis.Symbols, symbol)
	}
	return rows.Err()
}

func loadRepositoryArtifacts(ctx context.Context, query repositorySnapshotQuerier, analysis *repositoryfacts.Analysis) error {
	rows, err := query.Query(ctx, `
		SELECT artifact_id, COALESCE(file_id, ''), kind, name, detail,
		       source_sha256, origin
		FROM repository_artifacts
		WHERE analysis_id=$1
		ORDER BY artifact_id ASC
	`, analysis.ID)
	if err != nil {
		return fmt.Errorf("load repository analysis artifacts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var artifact repositoryfacts.Artifact
		var detail []byte
		if err := rows.Scan(
			&artifact.ID, &artifact.FileID, &artifact.Kind, &artifact.Name, &detail,
			&artifact.SourceSHA256, &artifact.Origin,
		); err != nil {
			return fmt.Errorf("scan repository analysis artifact: %w", err)
		}
		if err := json.Unmarshal(detail, &artifact.Detail); err != nil {
			return fmt.Errorf("decode repository artifact %q detail: %w", artifact.ID, err)
		}
		artifact.Adapter = analysis.Adapter
		analysis.Artifacts = append(analysis.Artifacts, artifact)
	}
	return rows.Err()
}

func loadRepositoryEdges(ctx context.Context, query repositorySnapshotQuerier, analysis *repositoryfacts.Analysis) error {
	rows, err := query.Query(ctx, `
		SELECT edge_id, from_id, to_id, kind, evidence_file_id,
		       evidence_start_byte, evidence_end_byte, origin, confidence
		FROM repository_edges
		WHERE analysis_id=$1
		ORDER BY edge_id ASC
	`, analysis.ID)
	if err != nil {
		return fmt.Errorf("load repository analysis edges: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var edge repositoryfacts.Edge
		var evidenceFileID *string
		var startByte, endByte *int64
		if err := rows.Scan(
			&edge.ID, &edge.FromID, &edge.ToID, &edge.Kind, &evidenceFileID,
			&startByte, &endByte, &edge.Origin, &edge.Confidence,
		); err != nil {
			return fmt.Errorf("scan repository analysis edge: %w", err)
		}
		if evidenceFileID != nil {
			if startByte == nil || endByte == nil {
				return fmt.Errorf("repository analysis edge %q has incomplete evidence coordinates", edge.ID)
			}
			edge.EvidenceFileID = *evidenceFileID
			edge.EvidenceStartByte = *startByte
			edge.EvidenceEndByte = *endByte
		}
		edge.Adapter = analysis.Adapter
		analysis.Edges = append(analysis.Edges, edge)
	}
	return rows.Err()
}

func loadRepositoryDiagnostics(ctx context.Context, query repositorySnapshotQuerier, analysis *repositoryfacts.Analysis) error {
	rows, err := query.Query(ctx, `
		SELECT severity, subject, detail
		FROM repository_analysis_diagnostics
		WHERE analysis_id=$1
		ORDER BY diagnostic_index ASC
	`, analysis.ID)
	if err != nil {
		return fmt.Errorf("load repository analysis diagnostics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var diagnostic repositoryfacts.AnalysisDiagnostic
		if err := rows.Scan(&diagnostic.Severity, &diagnostic.Subject, &diagnostic.Detail); err != nil {
			return fmt.Errorf("scan repository analysis diagnostic: %w", err)
		}
		analysis.Diagnostics = append(analysis.Diagnostics, diagnostic)
	}
	return rows.Err()
}
