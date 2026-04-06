package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/xiaocaoooo/gallery-server/internal/apperr"
	"github.com/xiaocaoooo/gallery-server/internal/model"
	"github.com/xiaocaoooo/gallery-server/internal/port"
)

type TagService struct {
	store port.TagStore
}

func NewTagService(store port.TagStore) *TagService {
	return &TagService{store: store}
}

func (s *TagService) Create(ctx context.Context, name string) (model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Tag{}, apperr.Validationf("tag name is required")
	}
	if utf8.RuneCountInString(name) > 64 {
		return model.Tag{}, apperr.Validationf("tag name must be at most 64 characters")
	}
	return s.store.CreateTag(ctx, name)
}

func (s *TagService) List(ctx context.Context, q string, limit int) ([]model.Tag, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return s.store.ListTags(ctx, strings.TrimSpace(q), limit)
}
