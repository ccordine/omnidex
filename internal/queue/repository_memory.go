package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (r *Repository) AddMemoryChunk(ctx context.Context, source, kind, content string, tags []string, embedding []float64) (model.MemoryChunk, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.MemoryChunk{}, err
	}
	defer tx.Rollback(ctx)

	if source == "" {
		source = "manual"
	}
	kind = normalizeMemoryKind(kind)
	content = strings.TrimSpace(content)
	if content == "" {
		return model.MemoryChunk{}, fmt.Errorf("memory content is required")
	}

	var chunk model.MemoryChunk
	err = tx.QueryRow(ctx, `
		SELECT id, source, kind, content, created_at
		FROM memory_chunks
		WHERE kind = $1 AND content = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, kind, content).Scan(&chunk.ID, &chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.MemoryChunk{}, err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		if memoryKindAllowsSemanticCorrection(kind) && len(embedding) > 0 {
			var existingID int64
			var distance float64
			correctionErr := tx.QueryRow(ctx, `
				SELECT id, COALESCE(embedding <=> $2::vector, 10.0) AS distance
				FROM memory_chunks
				WHERE kind = $1
				  AND embedding IS NOT NULL
				ORDER BY embedding <=> $2::vector ASC
				LIMIT 1
			`, kind, vectorLiteral(embedding)).Scan(&existingID, &distance)
			if correctionErr != nil && !errors.Is(correctionErr, pgx.ErrNoRows) {
				return model.MemoryChunk{}, correctionErr
			}

			if correctionErr == nil && distance <= inferredMemoryCorrectionDistance {
				err = tx.QueryRow(ctx, `
					UPDATE memory_chunks
					SET source = $2, content = $3, embedding = $4::vector
					WHERE id = $1
					RETURNING id, source, kind, content, created_at
				`, existingID, source, content, vectorLiteral(embedding)).Scan(&chunk.ID, &chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
				if err != nil {
					return model.MemoryChunk{}, err
				}
			}
		}

		if chunk.ID == 0 {
			if len(embedding) > 0 {
				err = tx.QueryRow(ctx, `
				INSERT INTO memory_chunks (source, kind, content, embedding)
			VALUES ($1, $2, $3, $4::vector)
			RETURNING id, source, kind, content, created_at
		`, source, kind, content, vectorLiteral(embedding)).Scan(&chunk.ID, &chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
			} else {
				err = tx.QueryRow(ctx, `
			INSERT INTO memory_chunks (source, kind, content)
			VALUES ($1, $2, $3)
			RETURNING id, source, kind, content, created_at
		`, source, kind, content).Scan(&chunk.ID, &chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
			}
			if err != nil {
				return model.MemoryChunk{}, err
			}
		}
	}

	categories := inferMemoryCategories(kind, tags)
	cleaned := decorateMemoryTags(source, appendCleanTags(tags, memoryCategoryTags(categories)...))
	for _, tag := range cleaned {
		var tagID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO tags(name) VALUES ($1)
			ON CONFLICT(name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, tag).Scan(&tagID)
		if err != nil {
			return model.MemoryChunk{}, err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO memory_chunk_tags (memory_chunk_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT(memory_chunk_id, tag_id) DO NOTHING
		`, chunk.ID, tagID); err != nil {
			return model.MemoryChunk{}, err
		}
	}

	for _, category := range categories {
		var categoryID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO memory_categories(name) VALUES ($1)
			ON CONFLICT(name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, category).Scan(&categoryID)
		if err != nil {
			return model.MemoryChunk{}, err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO memory_chunk_categories (memory_chunk_id, category_id)
			VALUES ($1, $2)
			ON CONFLICT(memory_chunk_id, category_id) DO NOTHING
		`, chunk.ID, categoryID); err != nil {
			return model.MemoryChunk{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.MemoryChunk{}, err
	}

	return chunk, nil
}

func memoryKindAllowsSemanticCorrection(kind string) bool {
	switch normalizeMemoryKind(kind) {
	case model.MemoryKindEpisodic, model.MemoryKindReference:
		return false
	default:
		return true
	}
}

func (r *Repository) FindRelevantMemory(ctx context.Context, embedding []float64, tags []string, limit int) ([]model.MemoryMatch, error) {
	if limit <= 0 {
		limit = 8
	}
	tags = cleanTags(tags)
	categoryFilters := memoryCategoryFilters(tags)
	trustOrder := fmt.Sprintf(`
				COALESCE(MAX(CASE WHEN t.name = '%s' THEN 1 ELSE 0 END), 0) DESC,
				COALESCE(MAX(CASE WHEN t.name = '%s' THEN 1 ELSE 0 END), 0) DESC,
				CASE
					WHEN mc.source = 'manual' THEN 0
					WHEN mc.source LIKE 'job:%%:reviewed:durable' THEN 1
					WHEN mc.source LIKE 'job:%%:reviewed:approved' THEN 2
					WHEN mc.source LIKE 'job:%%:inferred:approved' THEN 3
					ELSE 4
				END ASC,`, model.MemoryTrustTagDurable, model.MemoryTrustTagApproved)

	var rows pgx.Rows
	var err error

	if len(embedding) > 0 {
		query := fmt.Sprintf(`
			SELECT
				mc.id,
				mc.kind,
				mc.content,
				mc.created_at,
				COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags,
				COALESCE(array_remove(array_agg(DISTINCT c.name), NULL), ARRAY[]::text[]) AS categories,
				COALESCE(1 - (mc.embedding <=> $1::vector), 0) AS score
			FROM memory_chunks mc
			LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id = mc.id
			LEFT JOIN tags t ON t.id = mct.tag_id
			LEFT JOIN memory_chunk_categories mcc ON mcc.memory_chunk_id = mc.id
			LEFT JOIN memory_categories c ON c.id = mcc.category_id
			WHERE (
				$2::text[] IS NULL
				OR cardinality($2::text[]) = 0
				OR EXISTS (
					SELECT 1
					FROM memory_chunk_tags fmct
					JOIN tags ft ON ft.id = fmct.tag_id
					WHERE fmct.memory_chunk_id = mc.id
					  AND ft.name = ANY($2)
				)
				OR EXISTS (
					SELECT 1
					FROM memory_chunk_categories fmcc
					JOIN memory_categories fc ON fc.id = fmcc.category_id
					WHERE fmcc.memory_chunk_id = mc.id
					  AND fc.name = ANY($4)
				)
			)
			GROUP BY mc.id
			ORDER BY
%s
				CASE mc.kind
					WHEN 'instruction' THEN 0
					WHEN 'procedural' THEN 1
					WHEN 'reference' THEN 2
					WHEN 'preference' THEN 3
					ELSE 4
				END ASC,
				CASE WHEN mc.embedding IS NULL THEN 1 ELSE 0 END,
				mc.embedding <=> $1::vector ASC,
				mc.created_at DESC,
				mc.id DESC
			LIMIT $3
		`, trustOrder)
		rows, err = r.pool.Query(ctx, query, vectorLiteral(embedding), tags, limit, categoryFilters)
	} else {
		query := fmt.Sprintf(`
			SELECT
				mc.id,
				mc.kind,
				mc.content,
				mc.created_at,
				COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags,
				COALESCE(array_remove(array_agg(DISTINCT c.name), NULL), ARRAY[]::text[]) AS categories,
				0.0 AS score
			FROM memory_chunks mc
			LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id = mc.id
			LEFT JOIN tags t ON t.id = mct.tag_id
			LEFT JOIN memory_chunk_categories mcc ON mcc.memory_chunk_id = mc.id
			LEFT JOIN memory_categories c ON c.id = mcc.category_id
			WHERE (
				$1::text[] IS NULL
				OR cardinality($1::text[]) = 0
				OR EXISTS (
					SELECT 1
					FROM memory_chunk_tags fmct
					JOIN tags ft ON ft.id = fmct.tag_id
					WHERE fmct.memory_chunk_id = mc.id
					  AND ft.name = ANY($1)
				)
				OR EXISTS (
					SELECT 1
					FROM memory_chunk_categories fmcc
					JOIN memory_categories fc ON fc.id = fmcc.category_id
					WHERE fmcc.memory_chunk_id = mc.id
					  AND fc.name = ANY($3)
				)
			)
			GROUP BY mc.id
			ORDER BY
%s
				CASE mc.kind
					WHEN 'instruction' THEN 0
					WHEN 'procedural' THEN 1
					WHEN 'reference' THEN 2
					WHEN 'preference' THEN 3
					ELSE 4
				END ASC
				,
				mc.created_at DESC,
				mc.id DESC
			LIMIT $2
		`, trustOrder)
		rows, err = r.pool.Query(ctx, query, tags, limit, categoryFilters)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := make([]model.MemoryMatch, 0, limit)
	for rows.Next() {
		var m model.MemoryMatch
		if err := rows.Scan(&m.ID, &m.Kind, &m.Content, &m.CreatedAt, &m.Tags, &m.Categories, &m.Score); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

func (r *Repository) ListMemoryCategories(ctx context.Context, limit int) ([]model.MemoryFacet, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT c.name, COUNT(mcc.memory_chunk_id)::bigint AS count
		FROM memory_categories c
		JOIN memory_chunk_categories mcc ON mcc.category_id = c.id
		GROUP BY c.id, c.name
		ORDER BY count DESC, c.name ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryFacets(rows)
}

func (r *Repository) ListMemoryTags(ctx context.Context, limit int) ([]model.MemoryFacet, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.name, COUNT(mct.memory_chunk_id)::bigint AS count
		FROM tags t
		JOIN memory_chunk_tags mct ON mct.tag_id = t.id
		GROUP BY t.id, t.name
		ORDER BY count DESC, t.name ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryFacets(rows)
}

func (r *Repository) BackfillMemoryCategories(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `
		SELECT
			mc.id,
			mc.kind,
			COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags
		FROM memory_chunks mc
		LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id = mc.id
		LEFT JOIN tags t ON t.id = mct.tag_id
		WHERE NOT EXISTS (
			SELECT 1
			FROM memory_chunk_categories existing
			WHERE existing.memory_chunk_id = mc.id
		)
		GROUP BY mc.id
		ORDER BY mc.id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type pendingMemoryCategory struct {
		id         int64
		categories []string
	}
	pending := []pendingMemoryCategory{}
	for rows.Next() {
		var id int64
		var kind string
		var tags []string
		if err := rows.Scan(&id, &kind, &tags); err != nil {
			return err
		}
		pending = append(pending, pendingMemoryCategory{id: id, categories: inferMemoryCategories(kind, tags)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, item := range pending {
		for _, category := range item.categories {
			var categoryID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO memory_categories(name) VALUES ($1)
				ON CONFLICT(name) DO UPDATE SET name = EXCLUDED.name
				RETURNING id
			`, category).Scan(&categoryID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO memory_chunk_categories (memory_chunk_id, category_id)
				VALUES ($1, $2)
				ON CONFLICT(memory_chunk_id, category_id) DO NOTHING
			`, item.id, categoryID); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func scanMemoryFacets(rows pgx.Rows) ([]model.MemoryFacet, error) {
	facets := []model.MemoryFacet{}
	for rows.Next() {
		var facet model.MemoryFacet
		if err := rows.Scan(&facet.Name, &facet.Count); err != nil {
			return nil, err
		}
		facets = append(facets, facet)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return facets, nil
}

func (r *Repository) CreateChannel(ctx context.Context, channel model.Channel) (model.Channel, error) {
	channel.ID = normalizeChannelID(channel.ID, channel.Name)
	if channel.ID == "" {
		return model.Channel{}, fmt.Errorf("channel id is required")
	}
	channel.Persona = normalizeChannelPersona(channel.Persona)
	channel.Tags = cleanTags(channel.Tags)
	if len(channel.Context) == 0 || !json.Valid(channel.Context) {
		channel.Context = json.RawMessage(`{}`)
	}
	var out model.Channel
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_channels (id, name, persona, system, provider, model, context, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			persona = EXCLUDED.persona,
			system = EXCLUDED.system,
			provider = EXCLUDED.provider,
			model = EXCLUDED.model,
			context = EXCLUDED.context,
			tags = EXCLUDED.tags,
			updated_at = NOW()
		RETURNING id, name, persona, system, provider, model, context, tags, created_at, updated_at
	`, channel.ID, strings.TrimSpace(channel.Name), channel.Persona, strings.TrimSpace(channel.System), strings.TrimSpace(channel.Provider), strings.TrimSpace(channel.Model), string(channel.Context), channel.Tags).Scan(
		&out.ID,
		&out.Name,
		&out.Persona,
		&out.System,
		&out.Provider,
		&out.Model,
		&out.Context,
		&out.Tags,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return model.Channel{}, err
	}
	return out, nil
}

func (r *Repository) GetChannel(ctx context.Context, id string) (model.Channel, error) {
	var out model.Channel
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, persona, system, provider, model, context, tags, created_at, updated_at
		FROM ai_channels
		WHERE id = $1
	`, strings.TrimSpace(id)).Scan(
		&out.ID,
		&out.Name,
		&out.Persona,
		&out.System,
		&out.Provider,
		&out.Model,
		&out.Context,
		&out.Tags,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return model.Channel{}, err
	}
	return out, nil
}

func (r *Repository) ListChannels(ctx context.Context, limit, offset int) ([]model.Channel, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, persona, system, provider, model, context, tags, created_at, updated_at
		FROM ai_channels
		ORDER BY updated_at DESC, id ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []model.Channel{}
	for rows.Next() {
		var ch model.Channel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Persona, &ch.System, &ch.Provider, &ch.Model, &ch.Context, &ch.Tags, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *Repository) AddChannelMessage(ctx context.Context, channelID, role, content string) (model.ChannelMessage, error) {
	role = normalizeChannelMessageRole(role)
	content = SanitizeUTF8Text(strings.TrimSpace(content))
	if strings.TrimSpace(channelID) == "" {
		return model.ChannelMessage{}, fmt.Errorf("channel id is required")
	}
	if content == "" {
		return model.ChannelMessage{}, fmt.Errorf("message content is required")
	}
	var msg model.ChannelMessage
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_channel_messages (channel_id, role, content)
		VALUES ($1, $2, $3)
		RETURNING id, channel_id, role, content, created_at
	`, channelID, role, content).Scan(&msg.ID, &msg.ChannelID, &msg.Role, &msg.Content, &msg.CreatedAt)
	if err != nil {
		return model.ChannelMessage{}, err
	}
	_, _ = r.pool.Exec(ctx, `UPDATE ai_channels SET updated_at = NOW() WHERE id = $1`, channelID)
	return msg, nil
}

func (r *Repository) ListChannelMessages(ctx context.Context, channelID string, limit int) ([]model.ChannelMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, channel_id, role, content, created_at
		FROM (
			SELECT id, channel_id, role, content, created_at
			FROM ai_channel_messages
			WHERE channel_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		) recent
		ORDER BY created_at ASC, id ASC
	`, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []model.ChannelMessage{}
	for rows.Next() {
		var msg model.ChannelMessage
		if err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func normalizeChannelID(id, fallback string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = strings.TrimSpace(fallback)
	}
	id = strings.ToLower(id)
	id = channelIDSanitizer.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-_.:")
	if len(id) > 96 {
		id = strings.Trim(id[:96], "-_.:")
	}
	return id
}

func normalizeChannelPersona(persona string) string {
	switch strings.ToLower(strings.TrimSpace(persona)) {
	case "instruct", "assistant", "chat", "":
		return "assistant"
	case "roleplay", "rp":
		return "roleplay"
	case "narrate", "story":
		return "narrate"
	default:
		return strings.ToLower(strings.TrimSpace(persona))
	}
}

func normalizeChannelMessageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "system", "tool":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "user"
	}
}

func decorateMemoryTags(source string, tags []string) []string {
	out := append([]string(nil), tags...)
	source = strings.ToLower(strings.TrimSpace(source))
	switch {
	case source == "", source == "manual":
		out = append(out, model.MemoryTrustTagDurable, "provenance:user")
	case strings.Contains(source, ":reviewed:durable"):
		out = append(out, model.MemoryTrustTagDurable, "provenance:reviewed")
	case strings.Contains(source, ":reviewed:approved"):
		out = append(out, model.MemoryTrustTagApproved, "provenance:reviewed")
	case strings.Contains(source, ":inferred:approved"):
		out = append(out, model.MemoryTrustTagApproved, "provenance:inferred")
	case strings.Contains(source, ":prompt"), strings.Contains(source, ":response"):
		out = append(out, "scope:session")
	}
	return cleanTags(out)
}
