package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server    ServerConfig
	Auth      AuthConfig
	Postgres  PostgresConfig
	Valkey    ValkeyConfig
	Imaginary ImaginaryConfig
	SeaweedFS SeaweedFSConfig
	Qdrant    QdrantConfig
	Upload    UploadConfig
}

type ServerConfig struct {
	Addr            string
	GinMode         string
	ShutdownTimeout time.Duration
}

type AuthConfig struct {
	ReadToken  string
	WriteToken string
}

type PostgresConfig struct {
	DSN           string
	MaxConns      int32
	AutoMigrate   bool
	MigrationsDir string
}

type ValkeyConfig struct {
	Addr     string
	Password string
	DB       int
	LockTTL  time.Duration
}

type ImaginaryConfig struct {
	BaseURL string
	Timeout time.Duration
}

type SeaweedFSConfig struct {
	MasterURL     string
	PublicURL     string
	UploadTimeout time.Duration
}

type QdrantConfig struct {
	GRPCAddr            string
	APIKey              string
	Collection          string
	Timeout             time.Duration
	SimilarityThreshold float32
	SearchLimit         uint64
}

type UploadConfig struct {
	MaxBytes        int64
	DefaultPageSize int
	MaxPageSize     int
}

func Load() (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Addr:            getEnv("SERVER_ADDR", ":8080"),
			GinMode:         getEnv("GIN_MODE", "release"),
			ShutdownTimeout: getDurationEnv("SERVER_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Auth: AuthConfig{
			ReadToken:  os.Getenv("READ_TOKEN"),
			WriteToken: os.Getenv("WRITE_TOKEN"),
		},
		Postgres: PostgresConfig{
			DSN:           getEnv("POSTGRES_DSN", "postgres://gallery:gallery@postgres:5432/gallery?sslmode=disable"),
			MaxConns:      int32(getIntEnv("POSTGRES_MAX_CONNS", 10)),
			AutoMigrate:   getBoolEnv("POSTGRES_AUTO_MIGRATE", true),
			MigrationsDir: getEnv("POSTGRES_MIGRATIONS_DIR", "migrations"),
		},
		Valkey: ValkeyConfig{
			Addr:     getEnv("VALKEY_ADDR", "valkey:6379"),
			Password: os.Getenv("VALKEY_PASSWORD"),
			DB:       getIntEnv("VALKEY_DB", 0),
			LockTTL:  getDurationEnv("VALKEY_LOCK_TTL", 30*time.Second),
		},
		Imaginary: ImaginaryConfig{
			BaseURL: strings.TrimRight(getEnv("IMAGINARY_BASE_URL", "http://imaginary:9000"), "/"),
			Timeout: getDurationEnv("IMAGINARY_TIMEOUT", 60*time.Second),
		},
		SeaweedFS: SeaweedFSConfig{
			MasterURL:     strings.TrimRight(getEnv("SEAWEED_MASTER_URL", "http://seaweed-master:9333"), "/"),
			PublicURL:     strings.TrimRight(getEnv("SEAWEED_PUBLIC_URL", "http://seaweed-volume:8080"), "/"),
			UploadTimeout: getDurationEnv("SEAWEED_UPLOAD_TIMEOUT", 60*time.Second),
		},
		Qdrant: QdrantConfig{
			GRPCAddr:            getEnv("QDRANT_GRPC_ADDR", "qdrant:6334"),
			APIKey:              os.Getenv("QDRANT_API_KEY"),
			Collection:          getEnv("QDRANT_COLLECTION", "image_vectors"),
			Timeout:             getDurationEnv("QDRANT_TIMEOUT", 20*time.Second),
			SimilarityThreshold: getFloat32Env("QDRANT_SIMILARITY_THRESHOLD", 0.98),
			SearchLimit:         uint64(getIntEnv("QDRANT_SEARCH_LIMIT", 5)),
		},
		Upload: UploadConfig{
			MaxBytes:        getInt64Env("UPLOAD_MAX_BYTES", 32<<20),
			DefaultPageSize: getIntEnv("PAGINATION_DEFAULT_PAGE_SIZE", 20),
			MaxPageSize:     getIntEnv("PAGINATION_MAX_PAGE_SIZE", 100),
		},
	}

	if cfg.Imaginary.BaseURL == "" {
		return Config{}, fmt.Errorf("IMAGINARY_BASE_URL must not be empty")
	}
	if cfg.SeaweedFS.MasterURL == "" {
		return Config{}, fmt.Errorf("SEAWEED_MASTER_URL must not be empty")
	}
	if cfg.Qdrant.Collection == "" {
		return Config{}, fmt.Errorf("QDRANT_COLLECTION must not be empty")
	}
	if cfg.Upload.DefaultPageSize <= 0 {
		cfg.Upload.DefaultPageSize = 20
	}
	if cfg.Upload.MaxPageSize < cfg.Upload.DefaultPageSize {
		cfg.Upload.MaxPageSize = cfg.Upload.DefaultPageSize
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

func getBoolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getInt64Env(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getFloat32Env(key string, fallback float32) float32 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return fallback
	}
	return float32(parsed)
}
