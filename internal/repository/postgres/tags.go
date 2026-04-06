package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/xiaocaoooo/gallery-server/internal/apperr"
	"github.com/xiaocaoooo/gallery-server/internal/model"
)

func (s *Store) CreateTag(ctx context.Context, name string) (model.Tag, error) {
	const query = `
		INSERT INTO tags (name)
		VALUES ($1)
		RETURNING id, name, created_at
	`

	var tag model.Tag
	if err := s.db.QueryRow(ctx, query, name).Scan(&tag.ID, &tag.Name, &tag.CreatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.Tag{}, apperr.Conflictf("tag %q already exists (case-insensitive)", name)
		}
		return model.Tag{}, fmt.Errorf("create tag: %w", err)
	}
	return tag, nil
}

func (s *Store) ListTags(ctx context.Context, q string, limit int) ([]model.Tag, error) {
	const query = `
		SELECT id, name, created_at
		FROM tags
		WHERE $1 = '' OR name ILIKE '%' || $1 || '%'
		ORDER BY name ASC
		LIMIT $2
	`

	rows, err := s.db.Query(ctx, query, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
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
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	return tags, nil
}

func (s *Store) GetTagsByNamesInsensitive(ctx context.Context, names []string) ([]model.Tag, error) {
	if len(names) == 0 {
		return []model.Tag{}, nil
	}

	lowerNames := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		lowerNames = append(lowerNames, normalized)
	}
	if len(lowerNames) == 0 {
		return []model.Tag{}, nil
	}

	const query = `
		SELECT id, name, created_at
		FROM tags
		WHERE LOWER(name) = ANY($1)
		ORDER BY name ASC
	`

	rows, err := s.db.Query(ctx, query, lowerNames)
	if err != nil {
		return nil, fmt.Errorf("get tags by names: %w", err)
	}
	defer rows.Close()

	tags := make([]model.Tag, 0, len(lowerNames))
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags by names: %w", err)
	}

	return tags, nil
}

func scanTag(row interface{ Scan(dest ...any) error }) (model.Tag, error) {
	var tag model.Tag
	if err := row.Scan(&tag.ID, &tag.Name, &tag.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Tag{}, apperr.NotFoundf("tag not found")
		}
		return model.Tag{}, fmt.Errorf("scan tag: %w", err)
	}
	return tag, nil
}
