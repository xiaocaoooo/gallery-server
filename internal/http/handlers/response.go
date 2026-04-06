package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/xiaocaoooo/gallery-server/internal/apperr"
)

func writeError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}

func handleError(c *gin.Context, err error) {
	var duplicateErr *apperr.DuplicateImageConflictError

	switch {
	case err == nil:
		return
	case errors.Is(err, apperr.ErrValidation):
		writeError(c, http.StatusBadRequest, err.Error())
	case errors.As(err, &duplicateErr):
		payload := gin.H{"error": err.Error()}
		if duplicateErr.DuplicateImageID > 0 {
			payload["duplicate_image_id"] = duplicateErr.DuplicateImageID
		}
		c.AbortWithStatusJSON(http.StatusConflict, payload)
	case errors.Is(err, apperr.ErrConflict):
		writeError(c, http.StatusConflict, err.Error())
	case errors.Is(err, apperr.ErrNotFound):
		writeError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, apperr.ErrUnauthorized):
		writeError(c, http.StatusUnauthorized, err.Error())
	default:
		log.Printf("request failed: %v", err)
		writeError(c, http.StatusInternalServerError, "internal server error")
	}
}
