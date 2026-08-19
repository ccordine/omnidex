package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetDataSourceSchemaSnapshot(
	ctx context.Context,
	sourceID string,
) (datasource.SchemaSnapshot, bool, error) {
	if err := validateDataSourceID(sourceID); err != nil {
		return datasource.SchemaSnapshot{}, false, err
	}
	var exists bool
	var raw json.RawMessage
	if err := r.pool.QueryRow(ctx, `
		SELECT schema_catalog IS NOT NULL, COALESCE(schema_catalog, '{}'::jsonb)
		FROM data_sources
		WHERE id=$1
	`, sourceID).Scan(&exists, &raw); err != nil {
		return datasource.SchemaSnapshot{}, false, err
	}
	if !exists {
		return datasource.SchemaSnapshot{}, false, nil
	}
	snapshot, err := decodeExactDataSourceSchemaSnapshot(raw)
	if err != nil {
		return datasource.SchemaSnapshot{}, false, fmt.Errorf(
			"decode data source %q schema snapshot: %w", sourceID, err,
		)
	}
	if snapshot.SourceID != sourceID {
		return datasource.SchemaSnapshot{}, false, fmt.Errorf(
			"data source %q schema snapshot carries source identity %q", sourceID, snapshot.SourceID,
		)
	}
	return snapshot, true, nil
}

func (r *Repository) SaveDataSourceSchemaSnapshot(
	ctx context.Context,
	snapshot datasource.SchemaSnapshot,
) error {
	if err := validateDataSourceID(snapshot.SourceID); err != nil {
		return err
	}
	if err := validateDataSourceSchemaSnapshot(snapshot); err != nil {
		return err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	command, err := r.pool.Exec(ctx, `
		UPDATE data_sources
		SET schema_catalog=$2::jsonb, catalog_updated_at=$3, updated_at=NOW()
		WHERE id=$1
	`, snapshot.SourceID, payload, snapshot.CapturedAt)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteDataSourceSchemaSnapshot(ctx context.Context, sourceID string) error {
	if err := validateDataSourceID(sourceID); err != nil {
		return err
	}
	command, err := r.pool.Exec(ctx, `
		UPDATE data_sources
		SET schema_catalog=NULL, catalog_updated_at=NULL, updated_at=NOW()
		WHERE id=$1
	`, sourceID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func decodeExactDataSourceSchemaSnapshot(raw []byte) (datasource.SchemaSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot datasource.SchemaSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return datasource.SchemaSnapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return datasource.SchemaSnapshot{}, fmt.Errorf("schema snapshot contains trailing JSON")
	}
	if err := validateDataSourceSchemaSnapshot(snapshot); err != nil {
		return datasource.SchemaSnapshot{}, err
	}
	return snapshot, nil
}

func validateDataSourceSchemaSnapshot(snapshot datasource.SchemaSnapshot) error {
	if err := validateDataSourceID(snapshot.SourceID); err != nil {
		return err
	}
	if err := snapshot.ValidateIntegrity(); err != nil {
		return fmt.Errorf("data source schema snapshot integrity: %w", err)
	}
	return nil
}
