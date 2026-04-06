package app

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/xiaocaoooo/gallery-server/internal/clients/imaginary"
	pgclient "github.com/xiaocaoooo/gallery-server/internal/clients/postgres"
	qdrantclient "github.com/xiaocaoooo/gallery-server/internal/clients/qdrant"
	"github.com/xiaocaoooo/gallery-server/internal/clients/seaweedfs"
	"github.com/xiaocaoooo/gallery-server/internal/clients/valkey"
	"github.com/xiaocaoooo/gallery-server/internal/config"
	httpserver "github.com/xiaocaoooo/gallery-server/internal/http"
	handlerspkg "github.com/xiaocaoooo/gallery-server/internal/http/handlers"
	repositorypostgres "github.com/xiaocaoooo/gallery-server/internal/repository/postgres"
	"github.com/xiaocaoooo/gallery-server/internal/service"
)

type App struct {
	Router *gin.Engine

	postgresPool *pgclientPoolCloser
	locker       *valkey.Locker
	vectorStore  *qdrantclient.Client
}

type pgclientPoolCloser struct {
	close func()
}

func (c *pgclientPoolCloser) Close() error {
	if c != nil && c.close != nil {
		c.close()
	}
	return nil
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	gin.SetMode(cfg.Server.GinMode)

	pool, err := pgclient.New(ctx, cfg.Postgres)
	if err != nil {
		return nil, err
	}
	if cfg.Postgres.AutoMigrate {
		if err := pgclient.RunMigrations(ctx, pool, cfg.Postgres.MigrationsDir); err != nil {
			pool.Close()
			return nil, fmt.Errorf("run postgres migrations: %w", err)
		}
	}

	locker, err := valkey.New(ctx, cfg.Valkey)
	if err != nil {
		pool.Close()
		return nil, err
	}

	vectorStore, err := qdrantclient.New(ctx, cfg.Qdrant)
	if err != nil {
		locker.Close()
		pool.Close()
		return nil, err
	}
	if err := vectorStore.EnsureCollection(ctx); err != nil {
		vectorStore.Close()
		locker.Close()
		pool.Close()
		return nil, err
	}

	store := repositorypostgres.NewStore(pool)
	processor := imaginary.New(cfg.Imaginary)
	objects := seaweedfs.New(cfg.SeaweedFS)

	tagService := service.NewTagService(store)
	imageService := service.NewImageService(store, locker, processor, objects, vectorStore, cfg.Upload, cfg.Qdrant)

	tagHandler := handlerspkg.NewTagHandler(tagService)
	imageHandler := handlerspkg.NewImageHandler(imageService, cfg.Upload.MaxBytes)
	router := httpserver.NewRouter(cfg, tagHandler, imageHandler)

	return &App{
		Router:       router,
		postgresPool: &pgclientPoolCloser{close: pool.Close},
		locker:       locker,
		vectorStore:  vectorStore,
	}, nil
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.vectorStore != nil {
		_ = a.vectorStore.Close()
	}
	if a.locker != nil {
		_ = a.locker.Close()
	}
	if a.postgresPool != nil {
		_ = a.postgresPool.Close()
	}
	return nil
}
