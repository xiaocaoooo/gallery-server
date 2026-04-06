package seaweedfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/xiaocaoooo/gallery-server/internal/config"
)

type Client struct {
	masterURL  string
	publicURL  string
	httpClient *http.Client
}

type assignResponse struct {
	Count     int    `json:"count"`
	Error     string `json:"error"`
	FID       string `json:"fid"`
	PublicURL string `json:"publicUrl"`
	URL       string `json:"url"`
}

func New(cfg config.SeaweedFSConfig) *Client {
	return &Client{
		masterURL: cfg.MasterURL,
		publicURL: cfg.PublicURL,
		httpClient: &http.Client{
			Timeout: cfg.UploadTimeout,
		},
	}
}

func (c *Client) Upload(ctx context.Context, filename string, data []byte, contentType string) (string, error) {
	assign, err := c.assign(ctx)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create seaweed multipart file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("write seaweed multipart data: %w", err)
	}
	if err := writer.WriteField("cm", "false"); err != nil {
		return "", fmt.Errorf("write seaweed multipart field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close seaweed multipart writer: %w", err)
	}

	uploadBase := assign.PublicURL
	if uploadBase == "" {
		uploadBase = assign.URL
	}
	uploadURL := ensureHTTP(uploadBase)
	if !strings.HasSuffix(uploadURL, "/") {
		uploadURL += "/"
	}
	uploadURL += assign.FID

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", fmt.Errorf("create seaweed upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if contentType != "" {
		req.Header.Set("X-Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call seaweed upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("seaweed upload returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	return assign.FID, nil
}

func (c *Client) Delete(ctx context.Context, fid string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.FileURL(fid), nil)
	if err != nil {
		return fmt.Errorf("create seaweed delete request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call seaweed delete: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("seaweed delete returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

func (c *Client) FileURL(fid string) string {
	base := c.publicURL
	if base == "" {
		base = c.masterURL
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(fid, "/")
}

func (c *Client) assign(ctx context.Context) (assignResponse, error) {
	endpoint := strings.TrimRight(c.masterURL, "/") + path.Clean("/dir/assign")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return assignResponse{}, fmt.Errorf("create seaweed assign request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return assignResponse{}, fmt.Errorf("call seaweed assign: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return assignResponse{}, fmt.Errorf("seaweed assign returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var result assignResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return assignResponse{}, fmt.Errorf("decode seaweed assign response: %w", err)
	}
	if result.Error != "" {
		return assignResponse{}, fmt.Errorf("seaweed assign error: %s", result.Error)
	}
	if result.FID == "" {
		return assignResponse{}, fmt.Errorf("seaweed assign response missing fid")
	}
	return result, nil
}

func ensureHTTP(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return "http://" + value
}
