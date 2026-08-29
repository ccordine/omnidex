package queue

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/jackc/pgx/v5"
)

const (
	maxRepositorySymbolQueryBytes = 4 * 1024
	maxRepositorySymbolMatches    = 50
	maxRepositoryGraphSubjects    = 32
	maxRepositoryGraphEdges       = 200
)

func (r *Repository) SearchRepositorySymbols(
	ctx context.Context,
	projectID int64,
	analysisID, queryText string,
	limit int,
) ([]repositoryfacts.SymbolMatch, error) {
	if err := validateRepositoryQueryAuthority(ctx, r, projectID, analysisID); err != nil {
		return nil, err
	}
	queryText = strings.TrimSpace(queryText)
	if queryText == "" || len([]byte(queryText)) > maxRepositorySymbolQueryBytes {
		return nil, fmt.Errorf("repository symbol search requires 1-%d query bytes", maxRepositorySymbolQueryBytes)
	}
	if limit < 1 || limit > maxRepositorySymbolMatches {
		return nil, fmt.Errorf("repository symbol search limit must be between 1 and %d", maxRepositorySymbolMatches)
	}
	if err := requireCompleteRepositoryAnalysis(ctx, r.pool, projectID, analysisID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT s.symbol_id, s.file_id, s.kind, s.name, s.qualified_name,
		       s.signature, s.start_byte, s.end_byte, s.source_sha256,
		       s.origin, s.adapter_name, s.adapter_version, s.confidence,
		       CASE
		           WHEN lower(s.qualified_name)=lower($3) OR lower(s.name)=lower($3) THEN 'exact'
		           WHEN lower(s.qualified_name) LIKE lower($3) || '%' THEN 'prefix'
		           WHEN to_tsvector('simple', s.qualified_name || ' ' || s.signature)
		                @@ websearch_to_tsquery('simple', $3) THEN 'full_text'
		           ELSE 'trigram'
		       END AS match_kind,
		       (CASE
		           WHEN lower(s.qualified_name)=lower($3) OR lower(s.name)=lower($3) THEN 4.0
		           WHEN lower(s.qualified_name) LIKE lower($3) || '%' THEN 2.0
		           ELSE 0.0
		        END + similarity(s.qualified_name, $3) +
		        ts_rank(to_tsvector('simple', s.qualified_name || ' ' || s.signature),
		                websearch_to_tsquery('simple', $3))) AS rank
		FROM repository_symbols s
		JOIN repository_analyses a ON a.id=s.analysis_id AND a.snapshot_id=s.snapshot_id
		JOIN repository_snapshots rs ON rs.id=a.snapshot_id
		WHERE rs.project_id=$1 AND a.id=$2 AND a.complete=TRUE
		  AND (
		      lower(s.qualified_name)=lower($3) OR lower(s.name)=lower($3) OR
		      s.qualified_name % $3 OR
		      to_tsvector('simple', s.qualified_name || ' ' || s.signature)
		          @@ websearch_to_tsquery('simple', $3)
		  )
		ORDER BY rank DESC, s.symbol_id ASC
		LIMIT $4
	`, projectID, analysisID, queryText, limit)
	if err != nil {
		return nil, fmt.Errorf("search repository symbols: %w", err)
	}
	defer rows.Close()
	matches := make([]repositoryfacts.SymbolMatch, 0, limit)
	for rows.Next() {
		var match repositoryfacts.SymbolMatch
		if err := rows.Scan(
			&match.Symbol.ID, &match.Symbol.FileID, &match.Symbol.Kind, &match.Symbol.Name,
			&match.Symbol.QualifiedName, &match.Symbol.Signature, &match.Symbol.StartByte,
			&match.Symbol.EndByte, &match.Symbol.SourceSHA256, &match.Symbol.Origin,
			&match.Symbol.Adapter.Name, &match.Symbol.Adapter.Version, &match.Symbol.Confidence,
			&match.MatchKind, &match.Score,
		); err != nil {
			return nil, fmt.Errorf("scan repository symbol match: %w", err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search repository symbols: %w", err)
	}
	return matches, nil
}

func (r *Repository) RepositoryGraphNeighborhood(
	ctx context.Context,
	projectID int64,
	analysisID string,
	subjectIDs []string,
	limit int,
) (repositoryfacts.GraphNeighborhood, error) {
	if ctx == nil {
		return repositoryfacts.GraphNeighborhood{}, fmt.Errorf("repository graph query requires a context")
	}
	if len(subjectIDs) == 0 || len(subjectIDs) > maxRepositoryGraphSubjects {
		return repositoryfacts.GraphNeighborhood{}, fmt.Errorf("repository graph query requires 1-%d subject IDs", maxRepositoryGraphSubjects)
	}
	if err := validateRepositoryQueryAuthority(ctx, r, projectID, analysisID); err != nil {
		return repositoryfacts.GraphNeighborhood{}, err
	}
	if limit < 1 || limit > maxRepositoryGraphEdges {
		return repositoryfacts.GraphNeighborhood{}, fmt.Errorf("repository graph limit must be between 1 and %d", maxRepositoryGraphEdges)
	}
	clean, err := cleanRepositorySubjectIDs(subjectIDs)
	if err != nil {
		return repositoryfacts.GraphNeighborhood{}, err
	}
	if err := requireCompleteRepositoryAnalysis(ctx, r.pool, projectID, analysisID); err != nil {
		return repositoryfacts.GraphNeighborhood{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT e.edge_id, e.from_id, e.to_id, e.kind, e.evidence_file_id,
		       e.evidence_start_byte, e.evidence_end_byte, e.origin,
		       e.adapter_name, e.adapter_version, e.confidence
		FROM repository_edges e
		JOIN repository_analyses a ON a.id=e.analysis_id AND a.snapshot_id=e.snapshot_id
		JOIN repository_snapshots rs ON rs.id=a.snapshot_id
		WHERE rs.project_id=$1 AND a.id=$2 AND a.complete=TRUE
		  AND (e.from_id=ANY($3::text[]) OR e.to_id=ANY($3::text[]))
		ORDER BY e.edge_id ASC
		LIMIT $4
	`, projectID, analysisID, clean, limit)
	if err != nil {
		return repositoryfacts.GraphNeighborhood{}, fmt.Errorf("query repository graph: %w", err)
	}
	defer rows.Close()
	neighborhood := repositoryfacts.GraphNeighborhood{AnalysisID: analysisID, SubjectIDs: clean}
	for rows.Next() {
		edge, err := scanRepositoryEdge(rows)
		if err != nil {
			return repositoryfacts.GraphNeighborhood{}, err
		}
		neighborhood.Edges = append(neighborhood.Edges, edge)
	}
	if err := rows.Err(); err != nil {
		return repositoryfacts.GraphNeighborhood{}, fmt.Errorf("query repository graph: %w", err)
	}
	return neighborhood, nil
}

func validateRepositoryQueryAuthority(ctx context.Context, r *Repository, projectID int64, analysisID string) error {
	if ctx == nil {
		return fmt.Errorf("repository query requires a context")
	}
	if projectID < 1 {
		return fmt.Errorf("repository query requires a positive project ID")
	}
	if !validRepositoryOpaqueID(strings.TrimSpace(analysisID), "analysis_") {
		return fmt.Errorf("repository query requires a valid analysis ID")
	}
	if r == nil || r.pool == nil {
		return fmt.Errorf("repository query requires PostgreSQL")
	}
	return nil
}

func requireCompleteRepositoryAnalysis(ctx context.Context, query repositorySnapshotQuerier, projectID int64, analysisID string) error {
	var complete bool
	err := query.QueryRow(ctx, `
		SELECT a.complete
		FROM repository_analyses a
		JOIN repository_snapshots s ON s.id=a.snapshot_id
		WHERE s.project_id=$1 AND a.id=$2
	`, projectID, analysisID).Scan(&complete)
	if err != nil {
		return fmt.Errorf("authorize repository analysis %q: %w", analysisID, err)
	}
	if !complete {
		return fmt.Errorf("repository analysis %q is incomplete", analysisID)
	}
	return nil
}

func cleanRepositorySubjectIDs(values []string) ([]string, error) {
	out := append([]string(nil), values...)
	for _, value := range out {
		if !validRepositoryOpaqueID(value, "symbol_") && !validRepositoryOpaqueID(value, "artifact_") {
			return nil, fmt.Errorf("repository graph subject %q is not a valid symbol or artifact ID", value)
		}
	}
	sort.Strings(out)
	for index := 1; index < len(out); index++ {
		if out[index] == out[index-1] {
			return nil, fmt.Errorf("repository graph subject IDs must be unique")
		}
	}
	return out, nil
}

func validRepositoryOpaqueID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil && len(decoded) == 32
}

func scanRepositoryEdge(row pgx.Row) (repositoryfacts.Edge, error) {
	var edge repositoryfacts.Edge
	var evidenceFileID *string
	var startByte, endByte *int64
	if err := row.Scan(
		&edge.ID, &edge.FromID, &edge.ToID, &edge.Kind, &evidenceFileID,
		&startByte, &endByte, &edge.Origin, &edge.Adapter.Name,
		&edge.Adapter.Version, &edge.Confidence,
	); err != nil {
		return repositoryfacts.Edge{}, fmt.Errorf("scan repository graph edge: %w", err)
	}
	if evidenceFileID != nil {
		if startByte == nil || endByte == nil {
			return repositoryfacts.Edge{}, fmt.Errorf("repository graph edge %q has incomplete evidence coordinates", edge.ID)
		}
		edge.EvidenceFileID = *evidenceFileID
		edge.EvidenceStartByte = *startByte
		edge.EvidenceEndByte = *endByte
	}
	return edge, nil
}
