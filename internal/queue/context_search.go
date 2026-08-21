package queue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const (
	maxContextSearchTerms     = 3
	maxContextSearchTermBytes = 256
	maxContextSearchRecords   = 24
)

// ContextSearchRecord is an exact server-owned record returned by a fixed
// retrieval provider. It is not model control data; the worker replaces its
// source identity with a per-call opaque candidate ID before semantic
// relevance selection.
type ContextSearchRecord struct {
	Namespace string
	SourceID  string
	Content   string
}

func (r *Repository) SearchConversationContextRecords(
	ctx context.Context,
	job model.Job,
	terms []string,
	limit int,
) ([]ContextSearchRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("conversation context search requires PostgreSQL and context")
	}
	if err := validateContextSearchRequest(terms, limit); err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return []ContextSearchRecord{}, nil
	}
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("conversation context search requires exact channel message authority")
	}
	rows, err := r.pool.Query(ctx, `
		WITH query_terms AS (
			SELECT websearch_to_tsquery('simple', term) AS query
			FROM unnest($3::text[]) AS term
		)
		SELECT message.id,message.role,message.content,
		       MAX(ts_rank_cd(to_tsvector('simple',message.content),query_terms.query)) AS rank
		FROM ai_channel_messages AS message
		JOIN query_terms
		  ON to_tsvector('simple',message.content) @@ query_terms.query
		WHERE message.channel_id=$1 AND message.id<$2
		GROUP BY message.id,message.role,message.content
		ORDER BY rank DESC,message.id DESC
		LIMIT $4
	`, binding.ChannelID, binding.UserMessageID, terms, limit)
	if err != nil {
		return nil, fmt.Errorf("search exact channel transcript context: %w", err)
	}
	defer rows.Close()
	records := make([]ContextSearchRecord, 0, limit)
	for rows.Next() {
		var id int64
		var role model.ChannelMessageRole
		var content string
		var rank float32
		if err := rows.Scan(&id, &role, &content, &rank); err != nil {
			return nil, err
		}
		if err := model.ValidateChannelMessage(role, content); err != nil {
			return nil, fmt.Errorf("searched channel message %d is invalid: %w", id, err)
		}
		records = append(records, ContextSearchRecord{
			Namespace: "conversation_" + string(role),
			SourceID:  fmt.Sprintf("channel-message-%d", id),
			Content:   content,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *Repository) SearchRoleplayContextRecords(
	ctx context.Context,
	worldID string,
	viewpointID model.RoleplayCharacterID,
	sceneID string,
	createdBefore time.Time,
	terms []string,
	limit int,
) ([]ContextSearchRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("roleplay context search requires PostgreSQL and context")
	}
	if strings.TrimSpace(worldID) == "" || strings.TrimSpace(sceneID) == "" || createdBefore.IsZero() {
		return nil, fmt.Errorf("roleplay context search requires frozen world, scene, and time authority")
	}
	if err := viewpointID.Validate(); err != nil {
		return nil, err
	}
	if err := validateContextSearchRequest(terms, limit); err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return []ContextSearchRecord{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		WITH query_terms AS (
			SELECT websearch_to_tsquery('simple', term) AS query
			FROM unnest($5::text[]) AS term
		), candidates AS (
			SELECT 'fictional_canon'::text AS namespace,event.id AS source_id,
			       event.content,1 AS source_priority,event.ordinal AS source_ordinal
			FROM roleplay_character_knowledge AS knowledge
			JOIN roleplay_canon_events AS event
			  ON event.world_id=knowledge.world_id AND event.id=knowledge.canon_event_id
			WHERE knowledge.world_id=$1 AND knowledge.character_id=$2
			  AND event.created_at<=$4
			UNION ALL
			SELECT 'character_memory',memory.id,memory.content,2,memory.ordinal
			FROM roleplay_characters AS viewpoint
			JOIN roleplay_characters AS placement
			  ON placement.library_character_id=viewpoint.library_character_id
			JOIN roleplay_character_memories AS memory
			  ON memory.character_id=placement.id AND memory.world_id=placement.world_id
			WHERE viewpoint.world_id=$1 AND viewpoint.id=$2
			  AND memory.created_at<=$4
			UNION ALL
			SELECT 'simulation_event',transition.operation_id || '-' || event.ordinality::text,
			       event.content,3,transition.ordinal
			FROM roleplay_simulation_transitions AS transition
			CROSS JOIN LATERAL jsonb_array_elements_text(
				transition.result->'narrative_events'
			) WITH ORDINALITY AS event(content,ordinality)
			WHERE transition.world_id=$1 AND transition.scene_id=$3
			  AND transition.created_at<=$4
		), ranked AS (
			SELECT candidates.namespace,candidates.source_id,candidates.content,
			       candidates.source_priority,candidates.source_ordinal,
			       MAX(ts_rank_cd(to_tsvector('simple',candidates.content),query_terms.query)) AS rank
			FROM candidates
			JOIN query_terms
			  ON to_tsvector('simple',candidates.content) @@ query_terms.query
			GROUP BY candidates.namespace,candidates.source_id,candidates.content,
			         candidates.source_priority,candidates.source_ordinal
		)
		SELECT namespace,source_id,content
		FROM ranked
		ORDER BY rank DESC,source_priority ASC,source_ordinal DESC,source_id ASC
		LIMIT $6
	`, worldID, viewpointID, sceneID, createdBefore, terms, limit)
	if err != nil {
		return nil, fmt.Errorf("search frozen roleplay context: %w", err)
	}
	defer rows.Close()
	records := make([]ContextSearchRecord, 0, limit)
	for rows.Next() {
		var record ContextSearchRecord
		if err := rows.Scan(&record.Namespace, &record.SourceID, &record.Content); err != nil {
			return nil, err
		}
		if strings.TrimSpace(record.SourceID) == "" || strings.TrimSpace(record.Content) == "" {
			return nil, fmt.Errorf("roleplay context search returned invalid exact authority")
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// RankContextSearchRecords applies PostgreSQL full-text ranking to an already
// frozen in-memory authority set. It cannot acquire or mutate any source.
func (r *Repository) RankContextSearchRecords(
	ctx context.Context,
	terms []string,
	records []ContextSearchRecord,
	limit int,
) ([]ContextSearchRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("context record ranking requires PostgreSQL and context")
	}
	if err := validateContextSearchRequest(terms, limit); err != nil {
		return nil, err
	}
	if len(terms) == 0 || len(records) == 0 {
		return []ContextSearchRecord{}, nil
	}
	if len(records) > 128 {
		return nil, fmt.Errorf("context record ranking exceeds the 128-record frozen input bound")
	}
	namespaces := make([]string, len(records))
	sourceIDs := make([]string, len(records))
	contents := make([]string, len(records))
	for index, record := range records {
		if strings.TrimSpace(record.Namespace) == "" || strings.TrimSpace(record.SourceID) == "" ||
			strings.TrimSpace(record.Content) == "" {
			return nil, fmt.Errorf("context record %d is invalid", index)
		}
		namespaces[index], sourceIDs[index], contents[index] = record.Namespace, record.SourceID, record.Content
	}
	rows, err := r.pool.Query(ctx, `
		WITH records AS (
			SELECT namespace,source_id,content,ordinality
			FROM unnest($1::text[],$2::text[],$3::text[])
			     WITH ORDINALITY AS row(namespace,source_id,content,ordinality)
		), query_terms AS (
			SELECT websearch_to_tsquery('simple',term) AS query
			FROM unnest($4::text[]) AS term
		)
		SELECT records.namespace,records.source_id,records.content
		FROM records
		JOIN query_terms ON to_tsvector('simple',records.content) @@ query_terms.query
		GROUP BY records.namespace,records.source_id,records.content,records.ordinality
		ORDER BY MAX(ts_rank_cd(to_tsvector('simple',records.content),query_terms.query)) DESC,
		         records.ordinality ASC
		LIMIT $5
	`, namespaces, sourceIDs, contents, terms, limit)
	if err != nil {
		return nil, fmt.Errorf("rank frozen context records: %w", err)
	}
	defer rows.Close()
	ranked := make([]ContextSearchRecord, 0, limit)
	for rows.Next() {
		var record ContextSearchRecord
		if err := rows.Scan(&record.Namespace, &record.SourceID, &record.Content); err != nil {
			return nil, err
		}
		ranked = append(ranked, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ranked, nil
}

func validateContextSearchRequest(terms []string, limit int) error {
	if len(terms) > maxContextSearchTerms {
		return fmt.Errorf("context search exceeds the %d-term bound", maxContextSearchTerms)
	}
	seen := make(map[string]struct{}, len(terms))
	for index, term := range terms {
		if term == "" || term != strings.TrimSpace(term) || len(term) > maxContextSearchTermBytes {
			return fmt.Errorf("context search term %d must contain 1..%d trimmed bytes", index, maxContextSearchTermBytes)
		}
		identity := strings.ToLower(term)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("context search term %q is duplicated", term)
		}
		seen[identity] = struct{}{}
	}
	if limit < 1 || limit > maxContextSearchRecords {
		return fmt.Errorf("context search record limit must be within 1..%d", maxContextSearchRecords)
	}
	return nil
}
