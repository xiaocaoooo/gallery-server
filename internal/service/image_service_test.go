package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"github.com/xiaocaoooo/gallery-server/internal/apperr"
	"github.com/xiaocaoooo/gallery-server/internal/config"
	"github.com/xiaocaoooo/gallery-server/internal/model"
	"github.com/xiaocaoooo/gallery-server/internal/port"
)

type fakeImageStore struct {
	tags                 []model.Tag
	phashMatch           *model.Image
	images               []model.Image
	imageTags            map[int64][]model.Tag
	createImageErr       error
	linkTagsErr          error
	updateDescriptionErr error
	createdImages        []model.Image
	linkedTagIDs         []int32
	nextID               int64
}

func (f *fakeImageStore) GetTagsByNamesInsensitive(_ context.Context, names []string) ([]model.Tag, error) {
	result := make([]model.Tag, 0, len(names))
	for _, name := range names {
		for _, tag := range f.tags {
			if strings.EqualFold(tag.Name, name) {
				result = append(result, tag)
			}
		}
	}
	return result, nil
}

func (f *fakeImageStore) FindImageByPhash(context.Context, int64) (*model.Image, error) {
	return f.phashMatch, nil
}

func (f *fakeImageStore) GetImageByID(_ context.Context, id int64) (model.Image, error) {
	for _, image := range f.images {
		if image.ID == id {
			return image, nil
		}
	}
	return model.Image{}, apperr.NotFoundf("image not found")
}

func (f *fakeImageStore) UpdateImageDescription(_ context.Context, imageID int64, description string) (model.Image, error) {
	if f.updateDescriptionErr != nil {
		return model.Image{}, f.updateDescriptionErr
	}
	for i, image := range f.images {
		if image.ID == imageID {
			f.images[i].Description = description
			return f.images[i], nil
		}
	}
	return model.Image{}, apperr.NotFoundf("image not found")
}

func (f *fakeImageStore) ListImages(context.Context, model.ImageListFilter) ([]model.Image, error) {
	return f.images, nil
}

func (f *fakeImageStore) ListImageTags(_ context.Context, imageID int64) ([]model.Tag, error) {
	return f.imageTags[imageID], nil
}

func (f *fakeImageStore) WithTx(_ context.Context, fn func(port.ImageWriteStore) error) error {
	return fn(f)
}

func (f *fakeImageStore) CreateImage(_ context.Context, image model.Image) (model.Image, error) {
	if f.createImageErr != nil {
		return model.Image{}, f.createImageErr
	}
	if f.nextID == 0 {
		f.nextID = 1
	}
	image.ID = f.nextID
	f.nextID++
	f.createdImages = append(f.createdImages, image)
	return image, nil
}

func (f *fakeImageStore) LinkImageTags(_ context.Context, _ int64, tagIDs []int32) error {
	if f.linkTagsErr != nil {
		return f.linkTagsErr
	}
	f.linkedTagIDs = append(f.linkedTagIDs, tagIDs...)
	return nil
}

type fakeLocker struct{}

func (fakeLocker) Acquire(context.Context, string) (string, error) { return "token", nil }
func (fakeLocker) Release(context.Context, string, string) error   { return nil }

type fakeProcessor struct {
	converted []byte
}

func (f fakeProcessor) ConvertToLosslessWebP(context.Context, string, []byte) ([]byte, error) {
	return f.converted, nil
}
func (fakeProcessor) Render(context.Context, string, model.RenderParams) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
}

type fakeObjectStore struct {
	fid string
}

func (f fakeObjectStore) Upload(context.Context, string, []byte, string) (string, error) {
	return f.fid, nil
}
func (fakeObjectStore) Delete(context.Context, string) error { return nil }
func (fakeObjectStore) FileURL(fid string) string            { return "http://example.com/" + fid }

type fakeVectorStore struct {
	matches     []model.VectorMatch
	upsertedIDs []int64
	deletedIDs  []int64
}

func (fakeVectorStore) EnsureCollection(context.Context) error { return nil }
func (f fakeVectorStore) SearchSimilar(context.Context, []float32, float32, uint64) ([]model.VectorMatch, error) {
	return f.matches, nil
}
func (f *fakeVectorStore) Upsert(_ context.Context, imageID int64, _ []float32) error {
	f.upsertedIDs = append(f.upsertedIDs, imageID)
	return nil
}
func (f *fakeVectorStore) Delete(_ context.Context, imageID int64) error {
	f.deletedIDs = append(f.deletedIDs, imageID)
	return nil
}
func (fakeVectorStore) Close() error { return nil }

func TestUploadReturnsConflictOnPhashMatch(t *testing.T) {
	store := &fakeImageStore{phashMatch: &model.Image{ID: 1}}
	vectorStore := &fakeVectorStore{}
	service := NewImageService(store, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, vectorStore, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	_, err := service.Upload(context.Background(), model.UploadRequest{Filename: "test.png", Data: makeTestPNG(t)})
	if err == nil || !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestUploadReturnsConflictOnVectorMatch(t *testing.T) {
	store := &fakeImageStore{}
	vectorStore := &fakeVectorStore{matches: []model.VectorMatch{{ImageID: 42, Score: 0.99}}}
	service := NewImageService(store, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, vectorStore, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	_, err := service.Upload(context.Background(), model.UploadRequest{Filename: "test.png", Data: makeTestPNG(t)})
	if err == nil || !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestUploadForceSkipsDedupAndUsesNumericID(t *testing.T) {
	store := &fakeImageStore{tags: []model.Tag{{ID: 1, Name: "Cat"}}}
	vectorStore := &fakeVectorStore{matches: []model.VectorMatch{{ImageID: 42, Score: 0.99}}}
	service := NewImageService(store, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, vectorStore, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	response, err := service.Upload(context.Background(), model.UploadRequest{Filename: "test.png", Data: makeTestPNG(t), Force: true, TagNames: []string{"cat"}})
	if err != nil {
		t.Fatalf("expected upload to succeed, got %v", err)
	}
	if response.ID != 1 {
		t.Fatalf("expected numeric image id 1, got %d", response.ID)
	}
	if response.FID != "1,1" {
		t.Fatalf("expected fid to be set, got %+v", response)
	}
	if len(store.createdImages) != 1 {
		t.Fatalf("expected one created image, got %d", len(store.createdImages))
	}
	if len(vectorStore.upsertedIDs) != 1 || vectorStore.upsertedIDs[0] != 1 {
		t.Fatalf("expected vector upsert for image 1, got %+v", vectorStore.upsertedIDs)
	}
	if len(store.linkedTagIDs) != 1 || store.linkedTagIDs[0] != 1 {
		t.Fatalf("expected tag link to be recorded, got %+v", store.linkedTagIDs)
	}
}

func TestSetDescriptionUpdatesImageAndAllowsClearing(t *testing.T) {
	store := &fakeImageStore{
		images: []model.Image{{ID: 1, Description: "old description"}},
		imageTags: map[int64][]model.Tag{
			1: {{ID: 1, Name: "Cat"}},
		},
	}
	service := NewImageService(store, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, &fakeVectorStore{}, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	updated, err := service.SetDescription(context.Background(), "1", "  new description  ")
	if err != nil {
		t.Fatalf("expected SetDescription to succeed, got %v", err)
	}
	if updated.Description != "new description" {
		t.Fatalf("expected trimmed description, got %q", updated.Description)
	}
	if store.images[0].Description != "new description" {
		t.Fatalf("expected store to persist updated description, got %q", store.images[0].Description)
	}
	if len(updated.Tags) != 1 || updated.Tags[0].Name != "Cat" {
		t.Fatalf("expected tags to be returned, got %+v", updated.Tags)
	}

	cleared, err := service.SetDescription(context.Background(), "1", "   ")
	if err != nil {
		t.Fatalf("expected SetDescription to allow clearing description, got %v", err)
	}
	if cleared.Description != "" {
		t.Fatalf("expected description to be cleared, got %q", cleared.Description)
	}
}

func TestSetDescriptionReturnsNotFoundForMissingImage(t *testing.T) {
	service := NewImageService(&fakeImageStore{}, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, &fakeVectorStore{}, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	if _, err := service.SetDescription(context.Background(), "1", "desc"); err == nil || !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("expected not found error for missing image in SetDescription, got %v", err)
	}
}

func TestGetRenderAndSetDescriptionRejectNonNumericImageID(t *testing.T) {
	service := NewImageService(&fakeImageStore{}, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, &fakeVectorStore{}, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	if _, err := service.Get(context.Background(), "abc"); err == nil || !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("expected validation error for non-numeric id in Get, got %v", err)
	}
	if _, err := service.Render(context.Background(), "abc", model.RenderParams{}); err == nil || !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("expected validation error for non-numeric id in Render, got %v", err)
	}
	if _, err := service.SetDescription(context.Background(), "abc", "desc"); err == nil || !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("expected validation error for non-numeric id in SetDescription, got %v", err)
	}
}

func TestRenderAllowsOriginalWhenNoParamsProvided(t *testing.T) {
	store := &fakeImageStore{images: []model.Image{{ID: 1, FID: "1,1"}}}
	service := NewImageService(store, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, &fakeVectorStore{}, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	rendered, err := service.Render(context.Background(), "1", model.RenderParams{})
	if err != nil {
		t.Fatalf("expected render without params to return original image, got %v", err)
	}
	if rendered.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 status, got %d", rendered.StatusCode)
	}
}

func TestRenderAllowsFormatOrQualityWithoutDimensions(t *testing.T) {
	store := &fakeImageStore{images: []model.Image{{ID: 1, FID: "1,1", MimeType: "image/webp"}}}
	service := NewImageService(store, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, &fakeVectorStore{}, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	if _, err := service.Render(context.Background(), "1", model.RenderParams{Quality: 80}); err != nil {
		t.Fatalf("expected quality without dimensions to use original size, got %v", err)
	}
	if _, err := service.Render(context.Background(), "1", model.RenderParams{Format: "jpeg", Quality: 100}); err != nil {
		t.Fatalf("expected format conversion without dimensions to use original size, got %v", err)
	}
}

func TestRenderRejectsFitWithoutDimensionsOrInvalidFormat(t *testing.T) {
	store := &fakeImageStore{images: []model.Image{{ID: 1, FID: "1,1", MimeType: "image/webp"}}}
	service := NewImageService(store, fakeLocker{}, fakeProcessor{converted: makeTestPNG(t)}, fakeObjectStore{fid: "1,1"}, &fakeVectorStore{}, config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100}, config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5})

	if _, err := service.Render(context.Background(), "1", model.RenderParams{Fit: "contain"}); err == nil || !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("expected validation error when fit is set without dimensions, got %v", err)
	}
	if _, err := service.Render(context.Background(), "1", model.RenderParams{Format: "gif"}); err == nil || !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("expected validation error for unsupported render format, got %v", err)
	}
}

func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(10 + x), G: uint8(20 + y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
