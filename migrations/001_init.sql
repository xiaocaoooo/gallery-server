-- 画廊表
CREATE TABLE galleries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 画廊别名（全局唯一）
CREATE TABLE gallery_aliases (
    alias TEXT PRIMARY KEY,
    gallery_id UUID NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 图片表（全局资源）
CREATE TABLE images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sha256_hash BYTEA NOT NULL UNIQUE,
    sha256_hex TEXT GENERATED ALWAYS AS (ENCODE(sha256_hash, 'hex')) STORED,
    name TEXT UNIQUE,  -- NULL 表示未命名
    ext TEXT NOT NULL, -- 从文件头推断
    width INT NOT NULL,
    height INT NOT NULL,
    size_bytes BIGINT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 图片别名（全局唯一）
CREATE TABLE image_aliases (
    alias TEXT PRIMARY KEY,
    image_id UUID NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 画廊-图片关联
CREATE TABLE gallery_images (
    gallery_id UUID NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
    image_id UUID NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (gallery_id, image_id)
);

-- 感知哈希表
CREATE TABLE perceptual_hashes (
    image_id UUID PRIMARY KEY REFERENCES images(id) ON DELETE CASCADE,
    is_gif BOOLEAN NOT NULL DEFAULT FALSE,
    frame_count INT NOT NULL DEFAULT 1,
    ahash BIGINT NOT NULL,
    dhash_h BIGINT NOT NULL,
    dhash_v BIGINT NOT NULL,
    phash BIGINT NOT NULL,
    bucket1 INT NOT NULL,
    bucket2 INT NOT NULL,
    bucket3 INT NOT NULL,
    bucket4 INT NOT NULL,
    frame_hashes JSONB NOT NULL DEFAULT '[]', -- GIF 存储格式为 [[h, v], ...]
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 索引构建
CREATE INDEX idx_images_sha256_hex ON images(sha256_hex text_pattern_ops);
CREATE INDEX idx_images_name ON images(name);
CREATE INDEX idx_galleries_name ON galleries(name);
CREATE INDEX idx_perceptual_bucket1 ON perceptual_hashes(bucket1);
CREATE INDEX idx_perceptual_bucket2 ON perceptual_hashes(bucket2);
CREATE INDEX idx_perceptual_bucket3 ON perceptual_hashes(bucket3);
CREATE INDEX idx_perceptual_bucket4 ON perceptual_hashes(bucket4);
CREATE INDEX idx_gallery_images_gallery ON gallery_images(gallery_id);
CREATE INDEX idx_gallery_images_image ON gallery_images(image_id);
