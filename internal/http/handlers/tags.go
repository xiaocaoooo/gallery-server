package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xiaocaoooo/gallery-server/internal/model"
	"github.com/xiaocaoooo/gallery-server/internal/service"
)

type TagHandler struct {
	service *service.TagService
}

func NewTagHandler(service *service.TagService) *TagHandler {
	return &TagHandler{service: service}
}

func (h *TagHandler) Create(c *gin.Context) {
	var request model.CreateTagRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid JSON body")
		return
	}

	tag, err := h.service.Create(c.Request.Context(), request.Name)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, tag)
}

func (h *TagHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	tags, err := h.service.List(c.Request.Context(), c.Query("q"), limit)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": tags})
}
