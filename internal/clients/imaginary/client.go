package imaginary

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"

	"github.com/xiaocaoooo/gallery-server/internal/config"
	"github.com/xiaocaoooo/gallery-server/internal/imagehash"
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
	defer closeBody(resp.Body)

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
	if params.Format == "gif" {
		return c.renderGIF(ctx, sourceURL, params)
	}
	if params.Width == 0 && params.Height == 0 && params.Fit == "" && params.Quality == 0 && params.Format == "" {
		return c.fetchOriginal(ctx, sourceURL)
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
		defer closeBody(resp.Body)
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("imaginary render returned %d and response body could not be read: %w", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("imaginary render returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *Client) renderGIF(ctx context.Context, sourceURL string, params model.RenderParams) (*http.Response, error) {
	if params.Width > 0 || params.Height > 0 || params.Fit != "" || params.Quality > 0 {
		return nil, fmt.Errorf("gif render does not support width, height, fit, or quality")
	}

	body, contentType, err := c.fetchOriginalBytes(ctx, sourceURL)
	if err != nil {
		return nil, err
	}

	if contentType == "image/gif" {
		return responseFromBytes(body, "image/gif"), nil
	}
	if imagehash.IsAnimated(body) {
		return nil, fmt.Errorf("animated source cannot be exported as gif")
	}

	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decode source image for gif render: %w", err)
	}

	paletted := image.NewPaletted(img.Bounds(), palette.Plan9)
	draw.FloydSteinberg.Draw(paletted, img.Bounds(), img, image.Point{})

	var buf bytes.Buffer
	if err := gif.Encode(&buf, paletted, nil); err != nil {
		return nil, fmt.Errorf("encode gif render: %w", err)
	}
	return responseFromBytes(buf.Bytes(), "image/gif"), nil
}

func (c *Client) fetchOriginal(ctx context.Context, sourceURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create original image request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch original image: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer closeBody(resp.Body)
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("original image returned %d and response body could not be read: %w", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("original image returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *Client) fetchOriginalBytes(ctx context.Context, sourceURL string) ([]byte, string, error) {
	resp, err := c.fetchOriginal(ctx, sourceURL)
	if err != nil {
		return nil, "", err
	}
	defer closeBody(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read original image body: %w", err)
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = imagehash.DetectMimeType(body)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return body, contentType, nil
}

func responseFromBytes(body []byte, contentType string) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", contentType)
	header.Set("Content-Length", strconv.Itoa(len(body)))
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

func closeBody(body io.Closer) {
	if body == nil {
		return
	}
	_ = body.Close()
}
