DO $$
DECLARE
    images_id_type TEXT;
    image_tags_id_type TEXT;
    images_count BIGINT := 0;
    image_tags_count BIGINT := 0;
BEGIN
    SELECT data_type
    INTO images_id_type
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'images' AND column_name = 'id';

    SELECT data_type
    INTO image_tags_id_type
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'image_tags' AND column_name = 'image_id';

    IF images_id_type = 'bigint' AND image_tags_id_type = 'bigint' THEN
        RETURN;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'images') THEN
        EXECUTE 'SELECT COUNT(*) FROM images' INTO images_count;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'image_tags') THEN
        EXECUTE 'SELECT COUNT(*) FROM image_tags' INTO image_tags_count;
    END IF;

    IF images_count > 0 OR image_tags_count > 0 THEN
        RAISE EXCEPTION 'cannot switch image ids to BIGSERIAL while images or image_tags contain data; clear image data first';
    END IF;

    DROP TABLE IF EXISTS image_tags;
    DROP TABLE IF EXISTS images;

    CREATE TABLE images (
        id BIGSERIAL PRIMARY KEY,
        filename VARCHAR(255) NOT NULL,
        fid TEXT NOT NULL,
        file_size BIGINT NOT NULL,
        width INT,
        height INT,
        mime_type VARCHAR(32) NOT NULL DEFAULT 'image/webp',
        phash BIGINT NOT NULL,
        is_animated BOOLEAN NOT NULL DEFAULT FALSE,
        description TEXT NOT NULL DEFAULT '',
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE image_tags (
        image_id BIGINT NOT NULL REFERENCES images(id) ON DELETE CASCADE,
        tag_id INT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
        PRIMARY KEY (image_id, tag_id)
    );

    CREATE INDEX IF NOT EXISTS idx_images_phash ON images USING btree (phash);
    CREATE INDEX IF NOT EXISTS idx_image_tags_tag_id ON image_tags(tag_id);
END $$;
