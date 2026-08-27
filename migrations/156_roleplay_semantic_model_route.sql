BEGIN;

LOCK TABLE projects, jobs IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_trigger
		WHERE tgrelid='jobs'::regclass
		  AND tgname='jobs_chat_turn_binding_immutable'
		  AND NOT tgisinternal
	) OR to_regprocedure('reject_chat_turn_binding_update()') IS NULL THEN
		RAISE EXCEPTION 'inherited chat turn model-routing immutability authority is absent';
	END IF;

    IF EXISTS (
        SELECT 1
        FROM jobs
        WHERE status IN ('pending','running','waiting_input')
          AND jsonb_typeof(metadata->'model_config')='object'
          AND (metadata->'model_config') ?| ARRAY[
              'roleplay_canon_extraction_model',
              'roleplay_ongoing_action_model'
          ]
    ) THEN
        RAISE EXCEPTION
            'cannot retire split roleplay model routes while an affected job is active';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE (settings ? 'model_config')
          AND jsonb_typeof(settings->'model_config')<>'object'
    ) THEN
        RAISE EXCEPTION 'project model_config must be an object before roleplay route migration';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE jsonb_typeof(settings->'model_config')='object'
          AND NOT ((settings->'model_config') ? 'roleplay_semantic_model')
          AND (settings->'model_config') ? 'roleplay_canon_extraction_model'
          AND (settings->'model_config') ? 'roleplay_ongoing_action_model'
          AND (settings->'model_config'->'roleplay_canon_extraction_model')
              IS DISTINCT FROM
              (settings->'model_config'->'roleplay_ongoing_action_model')
    ) THEN
        RAISE EXCEPTION
            'conflicting split roleplay model routes require one explicit roleplay_semantic_model';
    END IF;

	IF EXISTS (
		SELECT 1
		FROM jobs
		WHERE jsonb_typeof(metadata->'model_config')='object'
		  AND NOT ((metadata->'model_config') ? 'roleplay_semantic_model')
		  AND (metadata->'model_config') ? 'roleplay_canon_extraction_model'
		  AND (metadata->'model_config') ? 'roleplay_ongoing_action_model'
		  AND (metadata->'model_config'->'roleplay_canon_extraction_model')
			  IS DISTINCT FROM
			  (metadata->'model_config'->'roleplay_ongoing_action_model')
	) THEN
		RAISE EXCEPTION
			'conflicting historical roleplay model routes require one explicit roleplay_semantic_model';
	END IF;

    IF EXISTS (
        SELECT 1
        FROM projects
        CROSS JOIN LATERAL (
            SELECT COALESCE(
                settings->'model_config'->'roleplay_semantic_model',
                settings->'model_config'->'roleplay_canon_extraction_model',
                settings->'model_config'->'roleplay_ongoing_action_model'
            ) AS selected
        ) AS route
        WHERE jsonb_typeof(settings->'model_config')='object'
          AND route.selected IS NOT NULL
          AND (
              jsonb_typeof(route.selected)<>'string' OR
              btrim(route.selected #>> '{}')=''
          )
    ) THEN
        RAISE EXCEPTION 'roleplay semantic model route must be one nonblank string';
    END IF;

	IF EXISTS (
		SELECT 1
		FROM jobs
		CROSS JOIN LATERAL (
			SELECT COALESCE(
				metadata->'model_config'->'roleplay_semantic_model',
				metadata->'model_config'->'roleplay_canon_extraction_model',
				metadata->'model_config'->'roleplay_ongoing_action_model'
			) AS selected
		) AS route
		WHERE jsonb_typeof(metadata->'model_config')='object'
		  AND route.selected IS NOT NULL
		  AND (
			  jsonb_typeof(route.selected)<>'string' OR
			  btrim(route.selected #>> '{}')=''
		  )
	) THEN
		RAISE EXCEPTION 'historical roleplay semantic model route must be one nonblank string';
	END IF;
END $$;

UPDATE projects
SET settings=jsonb_set(
    settings,
    '{model_config}',
    (
        (settings->'model_config')
            - 'roleplay_canon_extraction_model'
            - 'roleplay_ongoing_action_model'
    ) || jsonb_build_object(
        'roleplay_semantic_model',
        COALESCE(
            settings->'model_config'->'roleplay_semantic_model',
            settings->'model_config'->'roleplay_canon_extraction_model',
            settings->'model_config'->'roleplay_ongoing_action_model'
        )
    ),
    FALSE
)
WHERE jsonb_typeof(settings->'model_config')='object'
  AND (settings->'model_config') ?| ARRAY[
      'roleplay_semantic_model',
      'roleplay_canon_extraction_model',
      'roleplay_ongoing_action_model'
  ];

DROP TRIGGER jobs_chat_turn_binding_immutable ON jobs;

UPDATE jobs
SET metadata=jsonb_set(
	metadata,
	'{model_config}',
	(
		(metadata->'model_config')
			- 'roleplay_canon_extraction_model'
			- 'roleplay_ongoing_action_model'
	) || jsonb_build_object(
		'roleplay_semantic_model',
		COALESCE(
			metadata->'model_config'->'roleplay_semantic_model',
			metadata->'model_config'->'roleplay_canon_extraction_model',
			metadata->'model_config'->'roleplay_ongoing_action_model'
		)
	),
	FALSE
)
WHERE jsonb_typeof(metadata->'model_config')='object'
  AND (metadata->'model_config') ?| ARRAY[
	  'roleplay_semantic_model',
	  'roleplay_canon_extraction_model',
	  'roleplay_ongoing_action_model'
  ];

CREATE TRIGGER jobs_chat_turn_binding_immutable
BEFORE UPDATE OF pipeline,metadata ON jobs
FOR EACH ROW EXECUTE FUNCTION reject_chat_turn_binding_update();

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM projects
        WHERE jsonb_typeof(settings->'model_config')='object'
          AND (settings->'model_config') ?| ARRAY[
              'roleplay_canon_extraction_model',
              'roleplay_ongoing_action_model'
          ]
    ) THEN
        RAISE EXCEPTION 'split roleplay model routes remain after migration';
    END IF;
	IF EXISTS (
		SELECT 1
		FROM jobs
		WHERE jsonb_typeof(metadata->'model_config')='object'
		  AND (metadata->'model_config') ?| ARRAY[
			  'roleplay_canon_extraction_model',
			  'roleplay_ongoing_action_model'
		  ]
	) OR NOT EXISTS (
		SELECT 1 FROM pg_trigger
		WHERE tgrelid='jobs'::regclass
		  AND tgname='jobs_chat_turn_binding_immutable'
		  AND NOT tgisinternal
	) THEN
		RAISE EXCEPTION 'historical split roleplay routes or chat immutability postcondition failed';
	END IF;
END $$;

COMMIT;
