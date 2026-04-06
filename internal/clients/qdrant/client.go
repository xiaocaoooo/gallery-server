package qdrant

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	clientqdrant "github.com/qdrant/go-client/qdrant"
	"github.com/xiaocaoooo/gallery-server/internal/config"
	"github.com/xiaocaoooo/gallery-server/internal/model"
)

type Client struct {
	client     *clientqdrant.Client
	collection string
	timeout    uint64
}

func New(ctx context.Context, cfg config.QdrantConfig) (*Client, error) {
	host, port, useTLS, err := parseAddress(cfg.GRPCAddr)
	if err != nil {
		return nil, err
	}

	client, err := clientqdrant.NewClient(&clientqdrant.Config{
		Host:   host,
		Port:   port,
		APIKey: cfg.APIKey,
		UseTLS: useTLS,
	})
	if err != nil {
		return nil, fmt.Errorf("create qdrant client: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if _, err := client.HealthCheck(pingCtx); err != nil {
		client.Close()
		return nil, fmt.Errorf("qdrant health check: %w", err)
	}

	return &Client{client: client, collection: cfg.Collection, timeout: uint64(cfg.Timeout.Seconds())}, nil
}

func (c *Client) EnsureCollection(ctx context.Context) error {
	exists, err := c.client.CollectionExists(ctx, c.collection)
	if err != nil {
		return fmt.Errorf("check qdrant collection: %w", err)
	}
	if exists {
		return nil
	}

	if err := c.client.CreateCollection(ctx, &clientqdrant.CreateCollection{
		CollectionName: c.collection,
		VectorsConfig: clientqdrant.NewVectorsConfig(&clientqdrant.VectorParams{
			Size:     256,
			Distance: clientqdrant.Distance_Cosine,
		}),
	}); err != nil {
		return fmt.Errorf("create qdrant collection: %w", err)
	}
	return nil
}

func (c *Client) SearchSimilar(ctx context.Context, vector []float32, threshold float32, limit uint64) ([]model.VectorMatch, error) {
	if len(vector) == 0 {
		return []model.VectorMatch{}, nil
	}
	if limit == 0 {
		limit = 5
	}

	results, err := c.client.Query(ctx, &clientqdrant.QueryPoints{
		CollectionName: c.collection,
		Query:          clientqdrant.NewQuery(vector...),
		ScoreThreshold: &threshold,
		Limit:          &limit,
		Timeout:        &c.timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("query qdrant collection: %w", err)
	}

	matches := make([]model.VectorMatch, 0, len(results))
	for _, point := range results {
		if point.GetId() == nil {
			continue
		}
		imageID := int64(point.GetId().GetNum())
		if imageID == 0 && point.GetId().GetUuid() != "" {
			parsed, err := strconv.ParseInt(point.GetId().GetUuid(), 10, 64)
			if err == nil {
				imageID = parsed
			}
		}
		if imageID <= 0 {
			continue
		}
		matches = append(matches, model.VectorMatch{ImageID: imageID, Score: point.GetScore()})
	}
	return matches, nil
}

func (c *Client) Upsert(ctx context.Context, imageID int64, vector []float32) error {
	wait := true
	_, err := c.client.Upsert(ctx, &clientqdrant.UpsertPoints{
		CollectionName: c.collection,
		Wait:           &wait,
		Points: []*clientqdrant.PointStruct{
			{
				Id:      clientqdrant.NewIDNum(uint64(imageID)),
				Vectors: clientqdrant.NewVectors(vector...),
				Payload: clientqdrant.NewValueMap(map[string]any{"image_id": imageID}),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("upsert qdrant point: %w", err)
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, imageID int64) error {
	if imageID <= 0 {
		return nil
	}
	wait := true
	_, err := c.client.Delete(ctx, &clientqdrant.DeletePoints{
		CollectionName: c.collection,
		Points:         clientqdrant.NewPointsSelector(clientqdrant.NewIDNum(uint64(imageID))),
		Wait:           &wait,
	})
	if err != nil {
		return fmt.Errorf("delete qdrant point: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func parseAddress(addr string) (string, int, bool, error) {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return "", 0, false, fmt.Errorf("qdrant grpc address is empty")
	}

	useTLS := false
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", 0, false, fmt.Errorf("parse qdrant grpc address: %w", err)
		}
		useTLS = parsed.Scheme == "https" || parsed.Scheme == "grpcs"
		host := parsed.Hostname()
		port := parsed.Port()
		if port == "" {
			port = "6334"
		}
		parsedPort, err := strconv.Atoi(port)
		if err != nil {
			return "", 0, false, fmt.Errorf("parse qdrant grpc port: %w", err)
		}
		return host, parsedPort, useTLS, nil
	}

	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			return trimmed, 6334, false, nil
		}
		return "", 0, false, fmt.Errorf("parse qdrant grpc address: %w", err)
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, false, fmt.Errorf("parse qdrant grpc port: %w", err)
	}
	return host, parsedPort, false, nil
}
