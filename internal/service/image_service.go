package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xiaocaoooo/gallery-server/internal/apperr"
	"github.com/xiaocaoooo/gallery-server/internal/config"
	"github.com/xiaocaoooo/gallery-server/internal/imagehash"
	"github.com/xiaocaoooo/gallery-server/internal/model"
	"github.com/xiaocaoooo/gallery-server/internal/port"
)

type RenderedImage struct {
	StatusCode    int
	Header        http.Header
	Body          io.ReadCloser
	ContentLength int64
}

type ImageService struct {
	store       port.ImageStore
	locker      port.Locker
	processor   port.Processor
	objectStore port.ObjectStore
	vectorStore port.VectorStore
	maxBytes    int64
	defaultPage int
	maxPage     int
	threshold   float32
	searchLimit uint64
}

func NewImageService(store port.ImageStore, locker port.Locker, processor port.Processor, objectStore port.ObjectStore, vectorStore port.VectorStore, uploadCfg config.UploadConfig, qdrantCfg config.QdrantConfig) *ImageService {
	return &ImageService{
		store:       store,
		locker:      locker,
		processor:   processor,
		objectStore: objectStore,
		vectorStore: vectorStore,
		maxBytes:    uploadCfg.MaxBytes,
		defaultPage: uploadCfg.DefaultPageSize,
		maxPage:     uploadCfg.MaxPageSize,
		threshold:   qdrantCfg.SimilarityThreshold,
		searchLimit: qdrantCfg.SearchLimit,
	}
}

func (s *ImageService) Upload(ctx context.Context, req model.UploadRequest) (model.ImageWithTags, error) {
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		return model.ImageWithTags{}, apperr.Validationf("filename is required")
	}
	if len(req.Data) == 0 {
		return model.ImageWithTags{}, apperr.Validationf("file is required")
	}
	if s.maxBytes > 0 && int64(len(req.Data)) > s.maxBytes {
		return model.ImageWithTags{}, apperr.Validationf("file exceeds max upload size")
	}

	tags, err := s.resolveTags(ctx, req.TagNames)
	if err != nil {
		return model.ImageWithTags{}, err
	}

	rawDigest := sha256.Sum256(req.Data)
	lockKey := "gallery:upload:" + hex.EncodeToString(rawDigest[:])
	lockToken, err := s.locker.Acquire(ctx, lockKey)
	if err != nil {
		return model.ImageWithTags{}, apperr.Conflictf("an upload with the same payload is already being processed")
	}
	defer func() {
		if releaseErr := s.locker.Release(context.Background(), lockKey, lockToken); releaseErr != nil {
			log.Printf("release upload lock %s failed: %v", lockKey, releaseErr)
		}
	}()

	converted, err := s.processor.ConvertToLosslessWebP(ctx, filename, req.Data)
	if err != nil {
		return model.ImageWithTags{}, fmt.Errorf("convert image: %w", err)
	}

	features, err := imagehash.Analyze(converted)
	if err != nil {
		features, err = imagehash.Analyze(req.Data)
		if err != nil {
			return model.ImageWithTags{}, fmt.Errorf("analyze image features: %w", err)
		}
		features.IsAnimated = imagehash.IsAnimatedWebP(converted)
	}

	if !req.Force {
		existing, err := s.store.FindImageByPhash(ctx, features.PHash)
		if err != nil {
			return model.ImageWithTags{}, err
		}
		if existing != nil {
			return model.ImageWithTags{}, apperr.DuplicateImageConflictf(existing.ID, "an image with the same perceptual hash already exists")
		}

		matches, err := s.vectorStore.SearchSimilar(ctx, features.Vector, s.threshold, s.searchLimit)
		if err != nil {
			return model.ImageWithTags{}, fmt.Errorf("search similar vectors: %w", err)
		}
		for _, match := range matches {
			if match.Score >= s.threshold {
				return model.ImageWithTags{}, apperr.DuplicateImageConflictf(match.ImageID, "an image with a similar vector already exists")
			}
		}
	}

	fid, err := s.objectStore.Upload(ctx, filename, converted, "image/webp")
	if err != nil {
		return model.ImageWithTags{}, fmt.Errorf("upload image object: %w", err)
	}

	image := model.Image{
		Filename:   filename,
		FID:        fid,
		FileSize:   int64(len(converted)),
		Width:      features.Width,
		Height:     features.Height,
		MimeType:   "image/webp",
		PHash:      features.PHash,
		IsAnimated: features.IsAnimated,
		CreatedAt:  time.Now().UTC(),
	}

	tagIDs := make([]int32, 0, len(tags))
	for _, tag := range tags {
		tagIDs = append(tagIDs, tag.ID)
	}

	vectorInserted := false
	success := false
	createdImage := model.Image{}
	defer func() {
		if success {
			return
		}
		if vectorInserted {
			if deleteErr := s.vectorStore.Delete(context.Background(), createdImage.ID); deleteErr != nil {
				log.Printf("delete orphan vector for image %d failed: %v", createdImage.ID, deleteErr)
			}
		}
		if deleteErr := s.objectStore.Delete(context.Background(), fid); deleteErr != nil {
			log.Printf("delete orphan object %s failed: %v", fid, deleteErr)
		}
	}()

	err = s.store.WithTx(ctx, func(txStore port.ImageWriteStore) error {
		createdImage, err = txStore.CreateImage(ctx, image)
		if err != nil {
			return err
		}
		if len(tagIDs) > 0 {
			if err := txStore.LinkImageTags(ctx, createdImage.ID, tagIDs); err != nil {
				return err
			}
		}
		if err := s.vectorStore.Upsert(ctx, createdImage.ID, features.Vector); err != nil {
			return fmt.Errorf("upsert image vector: %w", err)
		}
		vectorInserted = true
		return nil
	})
	if err != nil {
		return model.ImageWithTags{}, err
	}

	success = true
	return model.ImageWithTags{Image: createdImage, Tags: tags}, nil
}

func (s *ImageService) List(ctx context.Context, filter model.ImageListFilter) (model.ImageListResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = s.defaultPage
	}
	if filter.PageSize > s.maxPage {
		filter.PageSize = s.maxPage
	}

	total, err := s.store.CountImages(ctx, filter)
	if err != nil {
		return model.ImageListResult{}, err
	}
	if total == 0 {
		return model.ImageListResult{
			Items:    []model.ImageWithTags{},
			Page:     filter.Page,
			PageSize: filter.PageSize,
			Total:    0,
		}, nil
	}

	images, err := s.store.ListImages(ctx, filter)
	if err != nil {
		return model.ImageListResult{}, err
	}

	items := make([]model.ImageWithTags, 0, len(images))
	for _, image := range images {
		tags, err := s.store.ListImageTags(ctx, image.ID)
		if err != nil {
			return model.ImageListResult{}, err
		}
		items = append(items, model.ImageWithTags{Image: image, Tags: tags})
	}

	return model.ImageListResult{
		Items:    items,
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Total:    total,
	}, nil
}

func (s *ImageService) Random(ctx context.Context, filter model.ImageListFilter) (model.ImageWithTags, error) {
	image, err := s.store.GetRandomImage(ctx, filter)
	if err != nil {
		return model.ImageWithTags{}, err
	}
	tags, err := s.store.ListImageTags(ctx, image.ID)
	if err != nil {
		return model.ImageWithTags{}, err
	}
	return model.ImageWithTags{Image: image, Tags: tags}, nil
}

func (s *ImageService) Get(ctx context.Context, id string) (model.ImageWithTags, error) {
	parsedID, err := parseImageID(id)
	if err != nil {
		return model.ImageWithTags{}, err
	}

	image, err := s.store.GetImageByID(ctx, parsedID)
	if err != nil {
		return model.ImageWithTags{}, err
	}
	tags, err := s.store.ListImageTags(ctx, parsedID)
	if err != nil {
		return model.ImageWithTags{}, err
	}

	return model.ImageWithTags{Image: image, Tags: tags}, nil
}

func (s *ImageService) SetDescription(ctx context.Context, id string, description string) (model.ImageWithTags, error) {
	parsedID, err := parseImageID(id)
	if err != nil {
		return model.ImageWithTags{}, err
	}

	description = strings.TrimSpace(description)
	image, err := s.store.UpdateImageDescription(ctx, parsedID, description)
	if err != nil {
		return model.ImageWithTags{}, err
	}
	tags, err := s.store.ListImageTags(ctx, image.ID)
	if err != nil {
		return model.ImageWithTags{}, err
	}

	return model.ImageWithTags{Image: image, Tags: tags}, nil
}

func (s *ImageService) Render(ctx context.Context, id string, params model.RenderParams) (*RenderedImage, error) {
	parsedID, err := parseImageID(id)
	if err != nil {
		return nil, err
	}
	if params.Width < 0 || params.Height < 0 {
		return nil, apperr.Validationf("render width and height must be positive")
	}
	if params.Quality < 0 || params.Quality > 100 {
		return nil, apperr.Validationf("render quality must be between 0 and 100")
	}
	params.Format, err = normalizeRenderFormat(params.Format)
	if err != nil {
		return nil, err
	}
	if params.Width == 0 && params.Height == 0 && params.Fit != "" {
		return nil, apperr.Validationf("render width or height is required when using fit")
	}
	if params.Fit != "" {
		switch params.Fit {
		case "cover", "contain", "fill", "inside", "outside":
		default:
			return nil, apperr.Validationf("unsupported render fit")
		}
	}

	image, err := s.store.GetImageByID(ctx, parsedID)
	if err != nil {
		return nil, err
	}

	if params.Width == 0 && params.Height == 0 && params.Format == "" && params.Quality > 0 {
		params.Format = formatFromMimeType(image.MimeType)
	}

	resp, err := s.processor.Render(ctx, s.objectStore.FileURL(image.FID), params)
	if err != nil {
		return nil, fmt.Errorf("render image: %w", err)
	}

	return &RenderedImage{
		StatusCode:    resp.StatusCode,
		Header:        resp.Header.Clone(),
		Body:          resp.Body,
		ContentLength: resp.ContentLength,
	}, nil
}

func (s *ImageService) resolveTags(ctx context.Context, names []string) ([]model.Tag, error) {
	unique := sanitizeTagNames(names)
	if len(unique) == 0 {
		return []model.Tag{}, nil
	}

	for _, name := range unique {
		if utf8.RuneCountInString(name) > 64 {
			return nil, apperr.Validationf("tag %q exceeds 64 characters", name)
		}
	}

	tags, err := s.store.GetTagsByNamesInsensitive(ctx, unique)
	if err != nil {
		return nil, err
	}
	if len(tags) != len(unique) {
		found := make(map[string]struct{}, len(tags))
		for _, tag := range tags {
			found[strings.ToLower(tag.Name)] = struct{}{}
		}
		missing := make([]string, 0)
		for _, name := range unique {
			if _, ok := found[strings.ToLower(name)]; !ok {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		return nil, apperr.Validationf("unknown tags: %s", strings.Join(missing, ", "))
	}

	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	return tags, nil
}

func sanitizeTagNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(names))
	result := make([]string, 0, len(names))
	for _, name := range names {
		for _, part := range strings.Split(name, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			normalized := strings.ToLower(trimmed)
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}

func parseImageID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Validationf("invalid image id")
	}
	return id, nil
}

func normalizeRenderFormat(raw string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	switch format {
	case "", "auto", "jpeg", "png", "webp":
		return format, nil
	case "jpg":
		return "jpeg", nil
	default:
		return "", apperr.Validationf("unsupported render format")
	}
}

func formatFromMimeType(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	default:
		return "auto"
	}
}
