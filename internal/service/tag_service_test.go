package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xiaocaoooo/gallery-server/internal/apperr"
	"github.com/xiaocaoooo/gallery-server/internal/model"
)

type fakeTagStore struct {
	createdName string
	listQ       string
	listLimit   int
	listTags    []model.Tag
	createdTags []model.Tag
}

func (f *fakeTagStore) CreateTag(_ context.Context, name string) (model.Tag, error) {
	for _, tag := range f.createdTags {
		if strings.EqualFold(tag.Name, name) {
			return model.Tag{}, apperr.Conflictf("tag %q already exists (case-insensitive)", name)
		}
	}
	f.createdName = name
	created := model.Tag{ID: int32(len(f.createdTags) + 1), Name: name, CreatedAt: time.Now().UTC()}
	f.createdTags = append(f.createdTags, created)
	return created, nil
}

func (f *fakeTagStore) ListTags(_ context.Context, q string, limit int) ([]model.Tag, error) {
	f.listQ = q
	f.listLimit = limit
	return f.listTags, nil
}

func (f *fakeTagStore) GetTagsByNamesInsensitive(context.Context, []string) ([]model.Tag, error) {
	return nil, nil
}

func TestTagServiceCreateValidation(t *testing.T) {
	store := &fakeTagStore{}
	service := NewTagService(store)

	if _, err := service.Create(context.Background(), "   "); err == nil || !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("expected validation error for blank name, got %v", err)
	}

	tooLong := "12345678901234567890123456789012345678901234567890123456789012345"
	if _, err := service.Create(context.Background(), tooLong); err == nil || !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("expected validation error for too long name, got %v", err)
	}

	tag, err := service.Create(context.Background(), "  Cat  ")
	if err != nil {
		t.Fatalf("expected valid tag create, got %v", err)
	}
	if store.createdName != "Cat" {
		t.Fatalf("expected trimmed tag name, got %q", store.createdName)
	}
	if tag.Name != "Cat" {
		t.Fatalf("expected returned tag name to match, got %q", tag.Name)
	}

	if _, err := service.Create(context.Background(), "cat"); err == nil || !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("expected case-insensitive duplicate conflict, got %v", err)
	}
}

func TestTagServiceListNormalizesInputAndLimit(t *testing.T) {
	store := &fakeTagStore{listTags: []model.Tag{{ID: 1, Name: "Cat"}}}
	service := NewTagService(store)

	items, err := service.List(context.Background(), "  cAt  ", 999)
	if err != nil {
		t.Fatalf("expected list to succeed, got %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if store.listQ != "cAt" {
		t.Fatalf("expected trimmed query, got %q", store.listQ)
	}
	if store.listLimit != 500 {
		t.Fatalf("expected clamped limit 500, got %d", store.listLimit)
	}
}
