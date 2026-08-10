package queue

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func attachMemoryTaxonomyTx(
	ctx context.Context,
	tx pgx.Tx,
	memoryID int64,
	source, kind string,
	tags []string,
) error {
	categories := inferMemoryCategories(kind, tags)
	cleaned := decorateMemoryTags(source, appendCleanTags(tags, memoryCategoryTags(categories)...))
	for _, tag := range cleaned {
		var tagID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO tags(name) VALUES ($1)
			ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name
			RETURNING id
		`, tag).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO memory_chunk_tags (memory_chunk_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT(memory_chunk_id, tag_id) DO NOTHING
		`, memoryID, tagID); err != nil {
			return err
		}
	}

	for _, category := range categories {
		var categoryID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO memory_categories(name) VALUES ($1)
			ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name
			RETURNING id
		`, category).Scan(&categoryID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO memory_chunk_categories (memory_chunk_id, category_id)
			VALUES ($1, $2)
			ON CONFLICT(memory_chunk_id, category_id) DO NOTHING
		`, memoryID, categoryID); err != nil {
			return err
		}
	}
	return nil
}
