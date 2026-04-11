package imaginary

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaocaoooo/gallery-server/internal/config"
	"github.com/xiaocaoooo/gallery-server/internal/model"
)

func TestRenderGIFEncodesStaticImage(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(makeTestPNG(t)); err != nil {
			t.Errorf("write png response: %v", err)
		}
	}))
	defer source.Close()

	client := New(config.ImaginaryConfig{BaseURL: "http://imaginary.example", Timeout: time.Second})
	resp, err := client.Render(context.Background(), source.URL, model.RenderParams{Format: "gif"})
	if err != nil {
		t.Fatalf("expected gif render to succeed, got %v", err)
	}
	defer closeBody(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read gif render body: %v", err)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/gif" {
		t.Fatalf("expected image/gif content type, got %q", got)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("decode rendered gif: %v", err)
	}
	if len(decoded.Image) != 1 {
		t.Fatalf("expected one gif frame, got %d", len(decoded.Image))
	}
}

func TestRenderGIFReturnsAnimatedGIFSourceUnchanged(t *testing.T) {
	animated := makeAnimatedGIF(t)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		if _, err := w.Write(animated); err != nil {
			t.Errorf("write gif response: %v", err)
		}
	}))
	defer source.Close()

	client := New(config.ImaginaryConfig{BaseURL: "http://imaginary.example", Timeout: time.Second})
	resp, err := client.Render(context.Background(), source.URL, model.RenderParams{Format: "gif"})
	if err != nil {
		t.Fatalf("expected animated gif render to succeed, got %v", err)
	}
	defer closeBody(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read animated gif render body: %v", err)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/gif" {
		t.Fatalf("expected image/gif content type, got %q", got)
	}
	if !bytes.Equal(body, animated) {
		t.Fatal("expected animated gif bytes to be returned unchanged")
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("decode animated gif render: %v", err)
	}
	if len(decoded.Image) != 2 {
		t.Fatalf("expected two gif frames, got %d", len(decoded.Image))
	}
}

func TestRenderGIFRejectsTransformParams(t *testing.T) {
	client := New(config.ImaginaryConfig{BaseURL: "http://imaginary.example", Timeout: time.Second})
	if _, err := client.Render(context.Background(), "http://example.com/image.gif", model.RenderParams{Format: "gif", Quality: 80}); err == nil {
		t.Fatal("expected gif render with quality to fail")
	}
}

func makeAnimatedGIF(t *testing.T) []byte {
	t.Helper()
	first := image.NewPaletted(image.Rect(0, 0, 4, 4), palette.Plan9)
	second := image.NewPaletted(image.Rect(0, 0, 4, 4), palette.Plan9)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			first.SetColorIndex(x, y, 1)
			second.SetColorIndex(x, y, 2)
		}
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{5, 5}}); err != nil {
		t.Fatalf("encode animated gif: %v", err)
	}
	return buf.Bytes()
}

func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(10 + x), G: uint8(20 + y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
