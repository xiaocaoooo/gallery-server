CREATE TABLE IF NOT EXISTS tags (
    id SERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS images (
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

CREATE TABLE IF NOT EXISTS image_tags (
    image_id BIGINT NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    tag_id INT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (image_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_images_phash ON images USING btree (phash);
CREATE INDEX IF NOT EXISTS idx_image_tags_tag_id ON image_tags(tag_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_name_lower_unique ON tags (LOWER(name));
