package queue

import (
	"context"
	"fmt"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) StoreRepositorySnapshot(
	ctx context.Context,
	projectID int64,
	snapshot repositoryfacts.Snapshot,
) error {
	if ctx == nil {
		return fmt.Errorf("store repository snapshot requires a context")
	}
	if projectID < 1 {
		return fmt.Errorf("store repository snapshot requires a positive project ID")
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("store repository snapshot: %w", err)
	}
	if r == nil || r.pool == nil {
		return fmt.Errorf("store repository snapshot requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin repository snapshot transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO repository_snapshots (
			id, project_id, schema_version, repository_id, root, head_commit,
			git_state_sha256, dirty, generated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO NOTHING
	`, snapshot.ID, projectID, snapshot.Schema, snapshot.RepositoryID, snapshot.Root,
		snapshot.HeadCommit, snapshot.GitStateSHA256, snapshot.Dirty, snapshot.GeneratedAt); err != nil {
		return fmt.Errorf("store repository snapshot identity: %w", err)
	}
	batch := &pgx.Batch{}
	for _, file := range snapshot.Files {
		batch.Queue(`
			INSERT INTO repository_files (
				snapshot_id, file_id, path, entry_kind, content_sha256, size_bytes,
				mode_bits, language, manifest_kind, is_test, is_generated, link_target
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (snapshot_id, path) DO NOTHING
		`, snapshot.ID, file.ID, file.Path, string(file.Kind), file.SHA256, file.Size,
			int(file.Mode), file.Language, file.Manifest, file.Test, file.Generated, file.LinkTarget)
	}
	for _, exclusion := range snapshot.Exclusions {
		batch.Queue(`
			INSERT INTO repository_exclusions (snapshot_id, path, reason)
			VALUES ($1,$2,$3)
			ON CONFLICT (snapshot_id, path) DO NOTHING
		`, snapshot.ID, exclusion.Path, string(exclusion.Reason))
	}
	results := tx.SendBatch(ctx, batch)
	if err := results.Close(); err != nil {
		return fmt.Errorf("store repository snapshot facts: %w", err)
	}
	stored, err := loadRepositorySnapshot(ctx, tx, projectID, snapshot.ID)
	if err != nil {
		return err
	}
	if stored.ID != snapshot.ID || len(stored.Files) != len(snapshot.Files) || len(stored.Exclusions) != len(snapshot.Exclusions) {
		return fmt.Errorf("stored repository snapshot %q does not match its exact submitted facts", snapshot.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repository snapshot: %w", err)
	}
	return nil
}

func (r *Repository) RepositorySnapshot(
	ctx context.Context,
	projectID int64,
	snapshotID string,
) (repositoryfacts.Snapshot, error) {
	if ctx == nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot requires a context")
	}
	if projectID < 1 {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot requires a positive project ID")
	}
	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot requires an ID")
	}
	if r == nil || r.pool == nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot requires PostgreSQL")
	}
	return loadRepositorySnapshot(ctx, r.pool, projectID, snapshotID)
}

func (r *Repository) LatestRepositorySnapshot(
	ctx context.Context,
	projectID int64,
) (repositoryfacts.Snapshot, error) {
	if ctx == nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load latest repository snapshot requires a context")
	}
	if projectID < 1 {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load latest repository snapshot requires a positive project ID")
	}
	if r == nil || r.pool == nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load latest repository snapshot requires PostgreSQL")
	}
	var snapshotID string
	if err := r.pool.QueryRow(ctx, `
		SELECT id
		FROM repository_snapshots
		WHERE project_id=$1
		ORDER BY generated_at DESC, id DESC
		LIMIT 1
	`, projectID).Scan(&snapshotID); err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load latest repository snapshot identity: %w", err)
	}
	return loadRepositorySnapshot(ctx, r.pool, projectID, snapshotID)
}

type repositorySnapshotQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadRepositorySnapshot(
	ctx context.Context,
	query repositorySnapshotQuerier,
	projectID int64,
	snapshotID string,
) (repositoryfacts.Snapshot, error) {
	snapshot := repositoryfacts.Snapshot{
		Files:      make([]repositoryfacts.File, 0),
		Exclusions: make([]repositoryfacts.Exclusion, 0),
	}
	err := query.QueryRow(ctx, `
		SELECT schema_version, id, repository_id, root, head_commit,
		       git_state_sha256, dirty, generated_at
		FROM repository_snapshots
		WHERE project_id=$1 AND id=$2
	`, projectID, snapshotID).Scan(
		&snapshot.Schema, &snapshot.ID, &snapshot.RepositoryID, &snapshot.Root,
		&snapshot.HeadCommit, &snapshot.GitStateSHA256, &snapshot.Dirty, &snapshot.GeneratedAt,
	)
	if err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot %q: %w", snapshotID, err)
	}
	snapshot.GeneratedAt = snapshot.GeneratedAt.UTC()
	rows, err := query.Query(ctx, `
		SELECT file_id, path, entry_kind, content_sha256, size_bytes, mode_bits,
		       language, manifest_kind, is_test, is_generated, link_target
		FROM repository_files
		WHERE snapshot_id=$1
		ORDER BY path COLLATE "C" ASC
	`, snapshotID)
	if err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot files: %w", err)
	}
	for rows.Next() {
		var file repositoryfacts.File
		var kind string
		var mode int
		if err := rows.Scan(
			&file.ID, &file.Path, &kind, &file.SHA256, &file.Size, &mode,
			&file.Language, &file.Manifest, &file.Test, &file.Generated, &file.LinkTarget,
		); err != nil {
			rows.Close()
			return repositoryfacts.Snapshot{}, fmt.Errorf("scan repository snapshot file: %w", err)
		}
		file.Kind = repositoryfacts.EntryKind(kind)
		file.Mode = uint32(mode)
		snapshot.Files = append(snapshot.Files, file)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot files: %w", err)
	}
	rows.Close()
	exclusions, err := query.Query(ctx, `
		SELECT path, reason
		FROM repository_exclusions
		WHERE snapshot_id=$1
		ORDER BY path COLLATE "C" ASC
	`, snapshotID)
	if err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot exclusions: %w", err)
	}
	defer exclusions.Close()
	for exclusions.Next() {
		var exclusion repositoryfacts.Exclusion
		var reason string
		if err := exclusions.Scan(&exclusion.Path, &reason); err != nil {
			return repositoryfacts.Snapshot{}, fmt.Errorf("scan repository snapshot exclusion: %w", err)
		}
		exclusion.Reason = repositoryfacts.ExclusionReason(reason)
		snapshot.Exclusions = append(snapshot.Exclusions, exclusion)
	}
	if err := exclusions.Err(); err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("load repository snapshot exclusions: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("stored repository snapshot %q is invalid: %w", snapshotID, err)
	}
	return snapshot, nil
}
