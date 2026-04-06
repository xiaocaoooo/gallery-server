package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/xiaocaoooo/gallery-server/internal/apperr"
	"github.com/xiaocaoooo/gallery-server/internal/model"
)

func (s *Store) FindImageByPhash(ctx context.Context, phash int64) (*model.Image, error) {
	const query = `
		SELECT id, filename, fid, file_size, width, height, mime_type, phash, is_animated, description, created_at
		FROM images
		WHERE phash = $1
		LIMIT 1
	`

	image, err := scanImage(s.db.QueryRow(ctx, query, phash))
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find image by phash: %w", err)
	}
	return &image, nil
}

func (s *Store) CreateImage(ctx context.Context, image model.Image) (model.Image, error) {
	const query = `
		INSERT INTO images (
			filename, fid, file_size, width, height, mime_type, phash, is_animated, description, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, filename, fid, file_size, width, height, mime_type, phash, is_animated, description, created_at
	`

	created, err := scanImage(s.db.QueryRow(ctx, query,
		image.Filename,
		image.FID,
		image.FileSize,
		image.Width,
		image.Height,
		image.MimeType,
		image.PHash,
		image.IsAnimated,
		image.Description,
		image.CreatedAt,
	))
	if err != nil {
		return model.Image{}, fmt.Errorf("create image: %w", err)
	}
	return created, nil
}

func (s *Store) LinkImageTags(ctx context.Context, imageID int64, tagIDs []int32) error {
	const query = `
		INSERT INTO image_tags (image_id, tag_id)
		VALUES ($1, $2)
		ON CONFLICT (image_id, tag_id) DO NOTHING
	`

	for _, tagID := range tagIDs {
		if _, err := s.db.Exec(ctx, query, imageID, tagID); err != nil {
			return fmt.Errorf("link image tag %d: %w", tagID, err)
		}
	}
	return nil
}

func (s *Store) GetImageByID(ctx context.Context, id int64) (model.Image, error) {
	const query = `
		SELECT id, filename, fid, file_size, width, height, mime_type, phash, is_animated, description, created_at
		FROM images
		WHERE id = $1
	`

	image, err := scanImage(s.db.QueryRow(ctx, query, id))
	if err != nil {
		return model.Image{}, fmt.Errorf("get image by id: %w", err)
	}
	return image, nil
}

func (s *Store) UpdateImageDescription(ctx context.Context, imageID int64, description string) (model.Image, error) {
	const query = `
		UPDATE images
		SET description = $2
		WHERE id = $1
		RETURNING id, filename, fid, file_size, width, height, mime_type, phash, is_animated, description, created_at
	`

	image, err := scanImage(s.db.QueryRow(ctx, query, imageID, description))
	if err != nil {
		return model.Image{}, fmt.Errorf("update image description: %w", err)
	}
	return image, nil
}

func (s *Store) ListImages(ctx context.Context, filter model.ImageListFilter) ([]model.Image, error) {
	offset := (filter.Page - 1) * filter.PageSize
	if offset < 0 {
		offset = 0
	}

	var (
		rows pgx.Rows
		err  error
	)

	if len(filter.Tags) == 0 {
		const query = `
			SELECT id, filename, fid, file_size, width, height, mime_type, phash, is_animated, description, created_at
			FROM images
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
		rows, err = s.db.Query(ctx, query, filter.PageSize, offset)
	} else {
		lowerTags := make([]string, 0, len(filter.Tags))
		seen := make(map[string]struct{}, len(filter.Tags))
		for _, tag := range filter.Tags {
			normalized := strings.ToLower(strings.TrimSpace(tag))
			if normalized == "" {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			lowerTags = append(lowerTags, normalized)
		}

		const query = `
			SELECT i.id, i.filename, i.fid, i.file_size, i.width, i.height, i.mime_type, i.phash, i.is_animated, i.description, i.created_at
			FROM images i
			WHERE i.id IN (
				SELECT it.image_id
				FROM image_tags it
				JOIN tags t ON t.id = it.tag_id
				WHERE LOWER(t.name) = ANY($1)
				GROUP BY it.image_id
				HAVING COUNT(DISTINCT LOWER(t.name)) = $2
			)
			ORDER BY i.created_at DESC
			LIMIT $3 OFFSET $4
		`
		rows, err = s.db.Query(ctx, query, lowerTags, len(lowerTags), filter.PageSize, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	defer rows.Close()

	images := make([]model.Image, 0, filter.PageSize)
	for rows.Next() {
		image, err := scanImage(rows)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate images: %w", err)
	}

	return images, nil
}

func (s *Store) ListImageTags(ctx context.Context, imageID int64) ([]model.Tag, error) {
	const query = `
		SELECT t.id, t.name, t.created_at
		FROM tags t
		JOIN image_tags it ON it.tag_id = t.id
		WHERE it.image_id = $1
		ORDER BY t.name ASC
	`

	rows, err := s.db.Query(ctx, query, imageID)
	if err != nil {
		return nil, fmt.Errorf("list image tags: %w", err)
	}
	defer rows.Close()

	tags := make([]model.Tag, 0)
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate image tags: %w", err)
	}

	return tags, nil
}

func scanImage(row interface{ Scan(dest ...any) error }) (model.Image, error) {
	var image model.Image
	if err := row.Scan(
		&image.ID,
		&image.Filename,
		&image.FID,
		&image.FileSize,
		&image.Width,
		&image.Height,
		&image.MimeType,
		&image.PHash,
		&image.IsAnimated,
		&image.Description,
		&image.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Image{}, apperr.NotFoundf("image not found")
		}
		return model.Image{}, fmt.Errorf("scan image: %w", err)
	}
	return image, nil
}
