package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xiaocaoooo/gallery-server/internal/apperr"
	"github.com/xiaocaoooo/gallery-server/internal/config"
	"github.com/xiaocaoooo/gallery-server/internal/http/handlers"
	"github.com/xiaocaoooo/gallery-server/internal/model"
	"github.com/xiaocaoooo/gallery-server/internal/port"
	"github.com/xiaocaoooo/gallery-server/internal/service"
)

type routerFakeTagStore struct {
	tags []model.Tag
}

func (f *routerFakeTagStore) CreateTag(_ context.Context, name string) (model.Tag, error) {
	for _, tag := range f.tags {
		if strings.EqualFold(tag.Name, name) {
			return model.Tag{}, apperr.Conflictf("tag %q already exists (case-insensitive)", name)
		}
	}
	tag := model.Tag{ID: int32(len(f.tags) + 1), Name: name, CreatedAt: time.Now().UTC()}
	f.tags = append(f.tags, tag)
	return tag, nil
}

func (f *routerFakeTagStore) ListTags(_ context.Context, _ string, _ int) ([]model.Tag, error) {
	return append([]model.Tag(nil), f.tags...), nil
}

func (f *routerFakeTagStore) GetTagsByNamesInsensitive(_ context.Context, names []string) ([]model.Tag, error) {
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

type routerFakeImageStore struct {
	images     map[int64]model.Image
	phashMatch *model.Image
}

func (f *routerFakeImageStore) GetTagsByNamesInsensitive(context.Context, []string) ([]model.Tag, error) {
	return nil, nil
}
func (f *routerFakeImageStore) FindImageByPhash(context.Context, int64) (*model.Image, error) {
	return f.phashMatch, nil
}
func (f *routerFakeImageStore) GetImageByID(_ context.Context, id int64) (model.Image, error) {
	image, ok := f.images[id]
	if !ok {
		return model.Image{}, apperr.NotFoundf("image not found")
	}
	return image, nil
}
func (f *routerFakeImageStore) UpdateImageDescription(_ context.Context, imageID int64, description string) (model.Image, error) {
	image, ok := f.images[imageID]
	if !ok {
		return model.Image{}, apperr.NotFoundf("image not found")
	}
	image.Description = description
	f.images[imageID] = image
	return image, nil
}
func (f *routerFakeImageStore) ListImages(context.Context, model.ImageListFilter) ([]model.Image, error) {
	items := make([]model.Image, 0, len(f.images))
	for _, image := range f.images {
		items = append(items, image)
	}
	return items, nil
}
func (*routerFakeImageStore) ListImageTags(context.Context, int64) ([]model.Tag, error) {
	return []model.Tag{}, nil
}
func (f *routerFakeImageStore) WithTx(_ context.Context, fn func(port.ImageWriteStore) error) error {
	return fn(routerFakeImageWriteStore{store: f})
}

type routerFakeImageWriteStore struct {
	store *routerFakeImageStore
}

func (f routerFakeImageWriteStore) CreateImage(_ context.Context, image model.Image) (model.Image, error) {
	image.ID = 1
	if f.store != nil {
		if f.store.images == nil {
			f.store.images = make(map[int64]model.Image)
		}
		f.store.images[image.ID] = image
	}
	return image, nil
}
func (routerFakeImageWriteStore) LinkImageTags(context.Context, int64, []int32) error {
	return nil
}

type routerFakeLocker struct{}

func (routerFakeLocker) Acquire(context.Context, string) (string, error) { return "token", nil }
func (routerFakeLocker) Release(context.Context, string, string) error   { return nil }

type routerFakeProcessor struct {
	converted []byte
}

func (f routerFakeProcessor) ConvertToLosslessWebP(context.Context, string, []byte) ([]byte, error) {
	return f.converted, nil
}
func (routerFakeProcessor) Render(context.Context, string, model.RenderParams) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
}

type routerFakeObjectStore struct{}

func (routerFakeObjectStore) Upload(context.Context, string, []byte, string) (string, error) {
	return "fid", nil
}
func (routerFakeObjectStore) Delete(context.Context, string) error { return nil }
func (routerFakeObjectStore) FileURL(string) string                { return "http://example.com/fid" }

type routerFakeVectorStore struct{}

func (routerFakeVectorStore) EnsureCollection(context.Context) error { return nil }
func (routerFakeVectorStore) SearchSimilar(context.Context, []float32, float32, uint64) ([]model.VectorMatch, error) {
	return []model.VectorMatch{}, nil
}
func (routerFakeVectorStore) Upsert(context.Context, int64, []float32) error { return nil }
func (routerFakeVectorStore) Delete(context.Context, int64) error            { return nil }
func (routerFakeVectorStore) Close() error                                   { return nil }

func TestRouterAuthModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter(config.Config{Auth: config.AuthConfig{ReadToken: "read-secret", WriteToken: "write-secret"}})

	t.Run("read route rejects missing token", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/tags", nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", recorder.Code)
		}
	})

	t.Run("read route accepts query read token", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/tags?token=read-secret", nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
	})

	t.Run("read route accepts bearer write token", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/tags", nil)
		req.Header.Set("Authorization", "Bearer write-secret")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
	})

	t.Run("write route rejects read token", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/tags", bytes.NewBufferString(`{"name":"Cat"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer read-secret")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", recorder.Code)
		}
	})

	t.Run("write route accepts query write token", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/tags?token=write-secret", bytes.NewBufferString(`{"name":"Cat"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", recorder.Code)
		}
		var tag model.Tag
		if err := json.Unmarshal(recorder.Body.Bytes(), &tag); err != nil {
			t.Fatalf("decode tag response: %v", err)
		}
		if tag.Name != "Cat" {
			t.Fatalf("expected created tag name Cat, got %q", tag.Name)
		}
	})

	t.Run("write route rejects case-insensitive duplicate tag", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/tags?token=write-secret", bytes.NewBufferString(`{"name":"cat"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", recorder.Code)
		}
	})
}

func TestRouterUploadReturnsDuplicateImageID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	imageStore := &routerFakeImageStore{
		images: map[int64]model.Image{
			1: {ID: 1, Filename: "seed.webp", FID: "fid", MimeType: "image/webp", Description: ""},
		},
		phashMatch: &model.Image{ID: 123},
	}
	router := newTestRouterWithImageStore(config.Config{Auth: config.AuthConfig{ReadToken: "read-secret", WriteToken: "write-secret"}}, imageStore)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", "duplicate.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fileWriter.Write(makeRouterTestPNG()); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/upload?token=write-secret", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d with body %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Error            string `json:"error"`
		DuplicateImageID int64  `json:"duplicate_image_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload conflict response: %v", err)
	}
	if response.DuplicateImageID != 123 {
		t.Fatalf("expected duplicate image id 123, got %d", response.DuplicateImageID)
	}
	if !strings.Contains(response.Error, "same perceptual hash") {
		t.Fatalf("expected duplicate conflict message, got %q", response.Error)
	}
}

func TestRouterSetImageDescriptionRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("rejects missing token", func(t *testing.T) {
		router := newTestRouter(config.Config{Auth: config.AuthConfig{ReadToken: "read-secret", WriteToken: "write-secret"}})
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/images/1/description", bytes.NewBufferString(`{"description":"hello"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", recorder.Code)
		}
	})

	t.Run("rejects read token", func(t *testing.T) {
		router := newTestRouter(config.Config{Auth: config.AuthConfig{ReadToken: "read-secret", WriteToken: "write-secret"}})
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/images/1/description", bytes.NewBufferString(`{"description":"hello"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer read-secret")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", recorder.Code)
		}
	})

	t.Run("rejects missing description field", func(t *testing.T) {
		router := newTestRouter(config.Config{Auth: config.AuthConfig{ReadToken: "read-secret", WriteToken: "write-secret"}})
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/images/1/description?token=write-secret", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", recorder.Code)
		}
	})

	t.Run("updates description with write token", func(t *testing.T) {
		router := newTestRouter(config.Config{Auth: config.AuthConfig{ReadToken: "read-secret", WriteToken: "write-secret"}})
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/images/1/description?token=write-secret", bytes.NewBufferString(`{"description":"hello world"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
		var image model.ImageWithTags
		if err := json.Unmarshal(recorder.Body.Bytes(), &image); err != nil {
			t.Fatalf("decode image response: %v", err)
		}
		if image.ID != 1 {
			t.Fatalf("expected image id 1, got %d", image.ID)
		}
		if image.Description != "hello world" {
			t.Fatalf("expected updated description, got %q", image.Description)
		}
	})
}

func TestRouterOpenAuthWhenTokensEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter(config.Config{})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/tags", nil)
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for open read route, got %d", recorder.Code)
	}
}

func newTestRouter(cfg config.Config) *gin.Engine {
	return newTestRouterWithImageStore(cfg, &routerFakeImageStore{images: map[int64]model.Image{
		1: {ID: 1, Filename: "seed.webp", FID: "fid", MimeType: "image/webp", Description: ""},
	}})
}

func newTestRouterWithImageStore(cfg config.Config, imageStore *routerFakeImageStore) *gin.Engine {
	tagStore := &routerFakeTagStore{tags: []model.Tag{{ID: 1, Name: "Seed", CreatedAt: time.Now().UTC()}}}
	tagHandler := handlers.NewTagHandler(service.NewTagService(tagStore))
	imageHandler := handlers.NewImageHandler(service.NewImageService(
		imageStore,
		routerFakeLocker{},
		routerFakeProcessor{converted: makeRouterTestPNG()},
		routerFakeObjectStore{},
		routerFakeVectorStore{},
		config.UploadConfig{MaxBytes: 1 << 20, DefaultPageSize: 20, MaxPageSize: 100},
		config.QdrantConfig{SimilarityThreshold: 0.98, SearchLimit: 5},
	), 1<<20)
	return NewRouter(cfg, tagHandler, imageHandler)
}

func makeRouterTestPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(10 + x), G: uint8(20 + y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
