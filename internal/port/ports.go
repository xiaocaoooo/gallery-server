package port

import (
	"context"
	"net/http"

	"github.com/xiaocaoooo/gallery-server/internal/model"
)

type TagStore interface {
	CreateTag(ctx context.Context, name string) (model.Tag, error)
	ListTags(ctx context.Context, q string, limit int) ([]model.Tag, error)
	GetTagsByNamesInsensitive(ctx context.Context, names []string) ([]model.Tag, error)
}

type ImageWriteStore interface {
	CreateImage(ctx context.Context, image model.Image) (model.Image, error)
	LinkImageTags(ctx context.Context, imageID int64, tagIDs []int32) error
}

type ImageStore interface {
	GetTagsByNamesInsensitive(ctx context.Context, names []string) ([]model.Tag, error)
	FindImageByPhash(ctx context.Context, phash int64) (*model.Image, error)
	GetImageByID(ctx context.Context, id int64) (model.Image, error)
	UpdateImageDescription(ctx context.Context, imageID int64, description string) (model.Image, error)
	CountImages(ctx context.Context, filter model.ImageListFilter) (int64, error)
	ListImages(ctx context.Context, filter model.ImageListFilter) ([]model.Image, error)
	GetRandomImage(ctx context.Context, filter model.ImageListFilter) (model.Image, error)
	ListImageTags(ctx context.Context, imageID int64) ([]model.Tag, error)
	WithTx(ctx context.Context, fn func(ImageWriteStore) error) error
}

type Locker interface {
	Acquire(ctx context.Context, key string) (string, error)
	Release(ctx context.Context, key, token string) error
}

type Processor interface {
	ConvertToLosslessWebP(ctx context.Context, filename string, data []byte) ([]byte, error)
	Render(ctx context.Context, sourceURL string, params model.RenderParams) (*http.Response, error)
}

type ObjectStore interface {
	Upload(ctx context.Context, filename string, data []byte, contentType string) (string, error)
	Delete(ctx context.Context, fid string) error
	FileURL(fid string) string
}

type VectorStore interface {
	EnsureCollection(ctx context.Context) error
	SearchSimilar(ctx context.Context, vector []float32, threshold float32, limit uint64) ([]model.VectorMatch, error)
	Upsert(ctx context.Context, imageID int64, vector []float32) error
	Delete(ctx context.Context, imageID int64) error
	Close() error
}
