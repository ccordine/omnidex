package queue

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

func dataSourceCatalogKey(sourceID string) string {
	return "data_source_catalog:" + strings.TrimSpace(sourceID)
}

func (r *Repository) GetDataSourceCatalog(ctx context.Context, sourceID string) (datasource.SchemaCatalog, bool, error) {
	raw, err := r.getWorkspaceJSON(ctx, dataSourceCatalogKey(sourceID))
	if err != nil {
		return datasource.SchemaCatalog{}, false, err
	}
	if len(raw) == 0 {
		return datasource.SchemaCatalog{}, false, nil
	}
	var catalog datasource.SchemaCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return datasource.SchemaCatalog{}, false, err
	}
	return catalog, len(catalog.Tables) > 0, nil
}

func (r *Repository) SaveDataSourceCatalog(ctx context.Context, catalog datasource.SchemaCatalog) error {
	if strings.TrimSpace(catalog.SourceID) == "" {
		return nil
	}
	if catalog.UpdatedAt.IsZero() {
		catalog.UpdatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO workspace_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW()
	`, dataSourceCatalogKey(catalog.SourceID), string(payload))
	return err
}

func (r *Repository) DeleteDataSourceCatalog(ctx context.Context, sourceID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM workspace_settings WHERE key = $1`, dataSourceCatalogKey(sourceID))
	return err
}
