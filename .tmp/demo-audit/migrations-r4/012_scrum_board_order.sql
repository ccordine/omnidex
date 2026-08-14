ALTER TABLE scrum_cards
    ADD COLUMN IF NOT EXISTS board_order INT NOT NULL DEFAULT 0;

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY project_id, column_name
               ORDER BY updated_at DESC, id ASC
           ) - 1 AS rn
    FROM scrum_cards
)
UPDATE scrum_cards AS c
SET board_order = r.rn
FROM ranked AS r
WHERE c.id = r.id;

CREATE INDEX IF NOT EXISTS idx_scrum_cards_project_column_order
    ON scrum_cards(project_id, column_name, board_order ASC);
