package queue

const scrumCardSummaryPageSQL = `
	SELECT card.id, card.project_id, card.title, card.description, card.column_name,
	       checklist_stats.done, jsonb_array_length(card.checklist),
	       jsonb_array_length(card.ref_files), card.channel_message_count,
	       test_stats.done, jsonb_array_length(card.test_criteria),
	       BTRIM(card.card_ticket) <> '', card.tags, card.flow_metrics,
	       card.job_id, card.play_state, card.queue_order, card.board_order,
	       card.created_at, card.updated_at,
	       checklist_stats.valid, ref_stats.valid, test_stats.valid
	FROM scrum_cards AS card
	CROSS JOIN LATERAL (
		SELECT COUNT(*) FILTER (WHERE item ->> 'done' = 'true')::integer AS done,
		       COALESCE(BOOL_AND(
		         jsonb_typeof(item) = 'object'
		         AND (NOT (item ? 'id') OR jsonb_typeof(item -> 'id') = 'string')
		         AND (NOT (item ? 'text') OR jsonb_typeof(item -> 'text') = 'string')
		         AND (NOT (item ? 'done') OR jsonb_typeof(item -> 'done') = 'boolean')
		       ), TRUE) AS valid
		FROM jsonb_array_elements(card.checklist) AS entry(item)
	) AS checklist_stats
	CROSS JOIN LATERAL (
		SELECT COALESCE(BOOL_AND(jsonb_typeof(item) = 'string'), TRUE) AS valid
		FROM jsonb_array_elements(card.ref_files) AS entry(item)
	) AS ref_stats
	CROSS JOIN LATERAL (
		SELECT COUNT(*) FILTER (WHERE item ->> 'done' = 'true')::integer AS done,
		       COALESCE(BOOL_AND(
		         jsonb_typeof(item) = 'object'
		         AND (NOT (item ? 'id') OR jsonb_typeof(item -> 'id') = 'string')
		         AND (NOT (item ? 'text') OR jsonb_typeof(item -> 'text') = 'string')
		         AND (NOT (item ? 'done') OR jsonb_typeof(item -> 'done') = 'boolean')
		       ), TRUE) AS valid
		FROM jsonb_array_elements(card.test_criteria) AS entry(item)
	) AS test_stats
	WHERE card.project_id = $1 AND ($2 = '' OR card.column_name = $2)
	ORDER BY
		CASE WHEN $2 = '' THEN card.updated_at END DESC,
		CASE WHEN $2 = '' THEN card.id END ASC,
		CASE WHEN $2 = 'assigned' AND card.play_state = 'queued' THEN 1 ELSE 0 END ASC,
		CASE WHEN $2 = 'assigned' AND card.play_state = 'queued' THEN card.queue_order ELSE 0 END ASC,
		CASE WHEN $2 = 'in_progress' AND card.play_state = 'running' THEN 0 ELSE 1 END ASC,
		card.board_order ASC, card.updated_at DESC, card.id ASC
	LIMIT $3 OFFSET $4`
