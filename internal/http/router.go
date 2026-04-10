package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/xiaocaoooo/gallery-server/internal/config"
	"github.com/xiaocaoooo/gallery-server/internal/http/handlers"
	"github.com/xiaocaoooo/gallery-server/internal/http/middleware"
)

func NewRouter(cfg config.Config, tagHandler *handlers.TagHandler, imageHandler *handlers.ImageHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), middleware.RequestID())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := router.Group("/v1")

	read := api.Group("")
	read.Use(middleware.RequireReadAuth(cfg.Auth))
	read.GET("/images", imageHandler.List)
	read.GET("/images/random", imageHandler.Random)
	read.GET("/images/:id", imageHandler.Get)
	read.GET("/images/:id/render", imageHandler.Render)
	read.GET("/tags", tagHandler.List)

	write := api.Group("")
	write.Use(middleware.RequireWriteAuth(cfg.Auth))
	write.POST("/images/upload", imageHandler.Upload)
	write.POST("/images/:id/description", imageHandler.SetDescription)
	write.POST("/tags", tagHandler.Create)

	return router
}
