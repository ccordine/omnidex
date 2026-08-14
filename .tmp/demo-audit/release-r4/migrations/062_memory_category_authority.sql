WITH uncategorized AS (
    SELECT chunks.id AS memory_chunk_id,
           lower(btrim(chunks.kind)) AS kind
    FROM memory_chunks chunks
    WHERE NOT EXISTS (
        SELECT 1 FROM memory_chunk_categories existing
        WHERE existing.memory_chunk_id=chunks.id
    )
),
clean_tags AS (
    SELECT DISTINCT uncategorized.memory_chunk_id,
           lower(btrim(tags.name)) AS tag
    FROM uncategorized
    JOIN memory_chunk_tags memberships
      ON memberships.memory_chunk_id=uncategorized.memory_chunk_id
    JOIN tags ON tags.id=memberships.tag_id
    WHERE btrim(tags.name)<>''
),
raw_categories AS (
    SELECT memory_chunk_id,
           CASE kind
               WHEN 'procedural' THEN 'strategy'
               WHEN 'reference' THEN 'reference'
               WHEN 'preference' THEN 'preference'
               WHEN 'instruction' THEN 'instruction'
           END AS category
    FROM uncategorized
    UNION ALL
    SELECT memory_chunk_id,'personal' FROM uncategorized WHERE kind='preference'
    UNION ALL
    SELECT memory_chunk_id,
           CASE
               WHEN tag LIKE 'category:%' THEN substr(tag,length('category:')+1)
               WHEN tag LIKE 'project:%' THEN 'project'
               WHEN tag LIKE 'session:%' OR tag LIKE 'channel:%' THEN 'personal'
               WHEN tag LIKE 'provider:%' THEN 'integration'
               WHEN tag LIKE 'query:%' OR tag IN ('research','web_search') THEN 'research'
               WHEN tag IN ('success-playbook','learned-skill') THEN 'strategy'
           END
    FROM clean_tags
),
clean_categories AS (
    SELECT memory_chunk_id,
           replace(replace(regexp_replace(
               lower(btrim(category)),'^category:','','g'
           ),'_','-'),' ','-') AS category
    FROM raw_categories
    WHERE category IS NOT NULL
),
normalized_categories AS (
    SELECT memory_chunk_id,
           CASE
               WHEN category IN ('personal','person','user') THEN 'personal'
               WHEN category IN ('project','codebase','workspace','repo','repository') THEN 'project'
               WHEN category IN ('language','languages','programming-language') THEN 'language'
               WHEN category IN ('database','db','sql','pgsql','postgres','postgresql') THEN 'database'
               WHEN category IN (
                   'infrastructure','infra','docker','container','deployment','devops'
               ) THEN 'infrastructure'
               WHEN category IN ('frontend','ui','react','vite') THEN 'frontend'
               WHEN category IN ('integration','api','provider','model-provider') THEN 'integration'
               WHEN category IN ('strategy','procedural','playbook','skill') THEN 'strategy'
               WHEN category IN ('reference','research','documentation','docs') THEN 'research'
               WHEN category IN (
                   'preference','instruction','verification','troubleshooting','security','general'
               ) THEN category
               WHEN category<>'' AND octet_length(category)<=40 THEN category
           END AS category
    FROM clean_categories
),
derived AS (
    SELECT DISTINCT memory_chunk_id,category
    FROM normalized_categories
    WHERE category IS NOT NULL
),
complete AS (
    SELECT memory_chunk_id,category FROM derived
    UNION ALL
    SELECT uncategorized.memory_chunk_id,'general'
    FROM uncategorized
    WHERE NOT EXISTS (
        SELECT 1 FROM derived WHERE derived.memory_chunk_id=uncategorized.memory_chunk_id
    )
),
inserted_categories AS (
    INSERT INTO memory_categories (name)
    SELECT DISTINCT category FROM complete
    ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name
    RETURNING id,name
)
INSERT INTO memory_chunk_categories (memory_chunk_id,category_id)
SELECT complete.memory_chunk_id,categories.id
FROM complete
JOIN inserted_categories categories ON categories.name=complete.category
ON CONFLICT (memory_chunk_id,category_id) DO NOTHING;
