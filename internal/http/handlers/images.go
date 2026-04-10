package handlers

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xiaocaoooo/gallery-server/internal/model"
	"github.com/xiaocaoooo/gallery-server/internal/service"
)

type ImageHandler struct {
	service        *service.ImageService
	maxUploadBytes int64
}

func NewImageHandler(service *service.ImageService, maxUploadBytes int64) *ImageHandler {
	return &ImageHandler{service: service, maxUploadBytes: maxUploadBytes}
}

func (h *ImageHandler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeError(c, http.StatusBadRequest, "missing multipart field: file")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		handleError(c, err)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		handleError(c, err)
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid multipart form")
		return
	}

	force, _ := strconv.ParseBool(c.DefaultPostForm("force", "false"))
	response, err := h.service.Upload(c.Request.Context(), model.UploadRequest{
		Filename: fileHeader.Filename,
		Data:     data,
		TagNames: form.Value["tags"],
		Force:    force,
	})
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response)
}

func (h *ImageHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.service.List(c.Request.Context(), model.ImageListFilter{
		Tags:     collectTagFilters(c),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *ImageHandler) Random(c *gin.Context) {
	image, err := h.service.Random(c.Request.Context(), model.ImageListFilter{
		Tags: collectTagFilters(c),
	})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, image)
}

func (h *ImageHandler) Get(c *gin.Context) {
	image, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, image)
}

func (h *ImageHandler) SetDescription(c *gin.Context) {
	var request model.UpdateImageDescriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if request.Description == nil {
		writeError(c, http.StatusBadRequest, "description is required")
		return
	}

	image, err := h.service.SetDescription(c.Request.Context(), c.Param("id"), *request.Description)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, image)
}

func (h *ImageHandler) Render(c *gin.Context) {
	width, _ := strconv.Atoi(firstNonEmpty(c.Query("w"), c.Query("width")))
	height, _ := strconv.Atoi(firstNonEmpty(c.Query("h"), c.Query("height")))
	quality, _ := strconv.Atoi(c.DefaultQuery("quality", "0"))

	rendered, err := h.service.Render(c.Request.Context(), c.Param("id"), model.RenderParams{
		Width:   width,
		Height:  height,
		Fit:     c.Query("fit"),
		Quality: quality,
		Format:  firstNonEmpty(c.Query("format"), c.Query("type")),
	})
	if err != nil {
		handleError(c, err)
		return
	}
	defer rendered.Body.Close()

	for key, values := range rendered.Header {
		if shouldForwardHeader(key) {
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
	}
	contentType := rendered.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/webp"
	}
	c.DataFromReader(rendered.StatusCode, rendered.ContentLength, contentType, rendered.Body, nil)
}

func collectTagFilters(c *gin.Context) []string {
	values := make([]string, 0)
	values = append(values, c.QueryArray("tag")...)
	values = append(values, c.QueryArray("tags")...)
	if q := strings.TrimSpace(c.Query("tags")); q != "" {
		values = append(values, q)
	}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			lowered := strings.ToLower(trimmed)
			if _, ok := seen[lowered]; ok {
				continue
			}
			seen[lowered] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shouldForwardHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Content-Type", "Content-Length", "Cache-Control", "ETag", "Last-Modified":
		return true
	default:
		return false
	}
}
