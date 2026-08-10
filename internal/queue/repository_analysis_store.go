package queue

import (
	"context"
	"encoding/json"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) StoreRepositoryAnalysis(
	ctx context.Context,
	projectID int64,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) error {
	if ctx == nil {
		return fmt.Errorf("store repository analysis requires a context")
	}
	if projectID < 1 {
		return fmt.Errorf("store repository analysis requires a positive project ID")
	}
	if err := analysis.Validate(snapshot); err != nil {
		return fmt.Errorf("store repository analysis: %w", err)
	}
	if r == nil || r.pool == nil {
		return fmt.Errorf("store repository analysis requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin repository analysis transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	storedSnapshot, err := loadRepositorySnapshot(ctx, tx, projectID, snapshot.ID)
	if err != nil {
		return fmt.Errorf("store repository analysis snapshot authority: %w", err)
	}
	if storedSnapshot.ID != snapshot.ID {
		return fmt.Errorf("store repository analysis snapshot authority changed")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO repository_analyses (
			id, snapshot_id, schema_version, adapter_name, adapter_version,
			complete, generated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO NOTHING
	`, analysis.ID, analysis.SnapshotID, analysis.Schema, analysis.Adapter.Name,
		analysis.Adapter.Version, analysis.Complete, analysis.GeneratedAt); err != nil {
		return fmt.Errorf("store repository analysis identity: %w", err)
	}
	if err := queueRepositoryAnalysisFacts(ctx, tx, analysis); err != nil {
		return err
	}
	stored, _, err := loadRepositoryAnalysis(ctx, tx, projectID, analysis.ID)
	if err != nil {
		return err
	}
	if stored.ID != analysis.ID || !sameAnalysisCounts(stored, analysis) {
		return fmt.Errorf("stored repository analysis %q does not match its exact submitted facts", analysis.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repository analysis: %w", err)
	}
	return nil
}

func queueRepositoryAnalysisFacts(ctx context.Context, tx pgx.Tx, analysis repositoryfacts.Analysis) error {
	batch := &pgx.Batch{}
	for index, diagnostic := range analysis.Diagnostics {
		batch.Queue(`
			INSERT INTO repository_analysis_diagnostics (
				analysis_id, diagnostic_index, severity, subject, detail
			)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (analysis_id, diagnostic_index) DO NOTHING
		`, analysis.ID, index, diagnostic.Severity, diagnostic.Subject, diagnostic.Detail)
	}
	for _, symbol := range analysis.Symbols {
		batch.Queue(`
			INSERT INTO repository_symbols (
				analysis_id, snapshot_id, symbol_id, file_id, kind, name, qualified_name,
				signature, start_byte, end_byte, source_sha256, origin,
				adapter_name, adapter_version, confidence
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (analysis_id, symbol_id) DO NOTHING
		`, analysis.ID, analysis.SnapshotID, symbol.ID, symbol.FileID, symbol.Kind,
			symbol.Name, symbol.QualifiedName, symbol.Signature, symbol.StartByte, symbol.EndByte,
			symbol.SourceSHA256, string(symbol.Origin), symbol.Adapter.Name,
			symbol.Adapter.Version, symbol.Confidence)
	}
	for _, artifact := range analysis.Artifacts {
		detail, err := json.Marshal(artifact.Detail)
		if err != nil {
			return fmt.Errorf("encode repository artifact %q detail: %w", artifact.ID, err)
		}
		batch.Queue(`
			INSERT INTO repository_artifacts (
				analysis_id, snapshot_id, artifact_id, file_id, kind, name, detail,
				source_sha256, origin, adapter_name, adapter_version
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (analysis_id, artifact_id) DO NOTHING
		`, analysis.ID, analysis.SnapshotID, artifact.ID, nullableString(artifact.FileID),
			artifact.Kind, artifact.Name, detail, artifact.SourceSHA256, string(artifact.Origin),
			artifact.Adapter.Name, artifact.Adapter.Version)
	}
	for _, edge := range analysis.Edges {
		batch.Queue(`
			INSERT INTO repository_edges (
				analysis_id, snapshot_id, edge_id, from_id, to_id, kind,
				evidence_file_id, evidence_start_byte, evidence_end_byte, origin,
				adapter_name, adapter_version, confidence
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (analysis_id, edge_id) DO NOTHING
		`, analysis.ID, analysis.SnapshotID, edge.ID, edge.FromID, edge.ToID, edge.Kind,
			nullableString(edge.EvidenceFileID), nullableEvidenceOffset(edge.EvidenceFileID, edge.EvidenceStartByte),
			nullableEvidenceOffset(edge.EvidenceFileID, edge.EvidenceEndByte), string(edge.Origin),
			edge.Adapter.Name, edge.Adapter.Version, edge.Confidence)
	}
	results := tx.SendBatch(ctx, batch)
	if err := results.Close(); err != nil {
		return fmt.Errorf("store repository analysis facts: %w", err)
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableEvidenceOffset(evidenceFileID string, value int64) any {
	if evidenceFileID == "" {
		return nil
	}
	return value
}

func sameAnalysisCounts(left, right repositoryfacts.Analysis) bool {
	return len(left.Symbols) == len(right.Symbols) &&
		len(left.Artifacts) == len(right.Artifacts) &&
		len(left.Edges) == len(right.Edges) &&
		len(left.Diagnostics) == len(right.Diagnostics)
}
