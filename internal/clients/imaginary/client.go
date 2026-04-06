package imaginary

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/xiaocaoooo/gallery-server/internal/config"
	"github.com/xiaocaoooo/gallery-server/internal/model"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(cfg config.ImaginaryConfig) *Client {
	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (c *Client) ConvertToLosslessWebP(ctx context.Context, filename string, data []byte) ([]byte, error) {
	endpoint := c.baseURL + "/convert?type=webp&quality=100&lossless=true&stripmeta=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create imaginary convert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if strings.TrimSpace(filename) != "" {
		req.Header.Set("X-Filename", filename)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call imaginary convert: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read imaginary convert response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("imaginary convert returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *Client) Render(ctx context.Context, sourceURL string, params model.RenderParams) (*http.Response, error) {
	if params.Width == 0 && params.Height == 0 && params.Fit == "" && params.Quality == 0 && params.Format == "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create original image request: %w", err)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch original image: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("original image returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return resp, nil
	}

	values := url.Values{}
	values.Set("url", sourceURL)
	endpointPath := "/resize"
	if params.Width == 0 && params.Height == 0 {
		endpointPath = "/convert"
	}
	if params.Width > 0 {
		values.Set("width", strconv.Itoa(params.Width))
	}
	if params.Height > 0 {
		values.Set("height", strconv.Itoa(params.Height))
	}
	if params.Quality > 0 {
		values.Set("quality", strconv.Itoa(params.Quality))
	}
	if params.Fit != "" {
		values.Set("fit", params.Fit)
	}
	if params.Format != "" {
		values.Set("type", params.Format)
	}

	endpoint := c.baseURL + endpointPath + "?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create imaginary render request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call imaginary render: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("imaginary render returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}
