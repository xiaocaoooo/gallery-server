DO $$
DECLARE
    conflicting_tags TEXT;
BEGIN
    SELECT string_agg(grouped.names, '; ' ORDER BY grouped.names)
    INTO conflicting_tags
    FROM (
        SELECT string_agg(name, ', ' ORDER BY name) AS names
        FROM tags
        GROUP BY LOWER(name)
        HAVING COUNT(*) > 1
    ) AS grouped;

    IF conflicting_tags IS NOT NULL THEN
        RAISE EXCEPTION 'cannot enable case-insensitive unique tags; conflicting existing tags: %', conflicting_tags;
    END IF;
END $$;

ALTER TABLE tags DROP CONSTRAINT IF EXISTS tags_name_key;
DROP INDEX IF EXISTS idx_tags_name_lower;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_name_lower_unique ON tags (LOWER(name));
