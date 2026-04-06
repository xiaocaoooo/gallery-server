package imagehash

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeStableOnSameImage(t *testing.T) {
	imageBytes := makeSolidPNG(t, color.NRGBA{R: 120, G: 40, B: 220, A: 255})

	first, err := Analyze(imageBytes)
	if err != nil {
		t.Fatalf("first analyze failed: %v", err)
	}
	second, err := Analyze(imageBytes)
	if err != nil {
		t.Fatalf("second analyze failed: %v", err)
	}

	if first.PHash != second.PHash {
		t.Fatalf("expected stable phash, got %d and %d", first.PHash, second.PHash)
	}
	if len(first.Vector) != 256 || len(second.Vector) != 256 {
		t.Fatalf("expected 256-d vector, got %d and %d", len(first.Vector), len(second.Vector))
	}
	for i := range first.Vector {
		if first.Vector[i] != second.Vector[i] {
			t.Fatalf("expected stable vector at index %d", i)
		}
	}
}

func TestIsAnimatedWebP(t *testing.T) {
	fixturePath := filepath.Join("testdata", "animated.webp")
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatalf("create testdata dir: %v", err)
	}
	data := append([]byte("RIFF____WEBPVP8X"), []byte{0x0A, 0x00, 0x00, 0x00, 0x02}...)
	data = append(data, make([]byte, 9)...)
	if err := os.WriteFile(fixturePath, data, 0o644); err != nil {
		t.Fatalf("write test fixture: %v", err)
	}
	loaded, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read test fixture: %v", err)
	}
	if !IsAnimatedWebP(loaded) {
		t.Fatal("expected animated webp detection to be true")
	}
}

func makeSolidPNG(t *testing.T, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
	var buf bytesBuffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

type bytesBuffer struct {
	buf []byte
}

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *bytesBuffer) Bytes() []byte {
	return b.buf
}
