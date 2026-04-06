package imagehash

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"

	_ "golang.org/x/image/webp"
)

type Features struct {
	Width      int
	Height     int
	PHash      int64
	Vector     []float32
	IsAnimated bool
}

func Analyze(data []byte) (Features, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Features{}, fmt.Errorf("decode image: %w", err)
	}

	phash, err := ComputePHash(img)
	if err != nil {
		return Features{}, err
	}
	vector, err := ComputeVector(img)
	if err != nil {
		return Features{}, err
	}

	bounds := img.Bounds()
	return Features{
		Width:      bounds.Dx(),
		Height:     bounds.Dy(),
		PHash:      phash,
		Vector:     vector,
		IsAnimated: IsAnimatedWebP(data),
	}, nil
}

func ComputePHash(img image.Image) (int64, error) {
	if img == nil {
		return 0, errors.New("nil image")
	}

	gray := resizeToGray(img, 32, 32)
	var input [32][32]float64
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			input[y][x] = float64(gray.GrayAt(x, y).Y)
		}
	}

	coefficients := dct2D(input)
	values := make([]float64, 0, 64)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			values = append(values, coefficients[y][x])
		}
	}

	if len(values) != 64 {
		return 0, errors.New("invalid phash coefficient length")
	}

	mean := 0.0
	for i := 1; i < len(values); i++ {
		mean += values[i]
	}
	mean /= float64(len(values) - 1)

	var hash uint64
	for i, value := range values {
		if value > mean {
			hash |= 1 << uint(i)
		}
	}

	return int64(hash), nil
}

func dct2D(input [32][32]float64) [32][32]float64 {
	const size = 32
	var output [size][size]float64

	for u := 0; u < size; u++ {
		for v := 0; v < size; v++ {
			sum := 0.0
			for x := 0; x < size; x++ {
				for y := 0; y < size; y++ {
					sum += input[x][y] *
						math.Cos((2*float64(x)+1)*float64(u)*math.Pi/(2*size)) *
						math.Cos((2*float64(y)+1)*float64(v)*math.Pi/(2*size))
				}
			}
			output[u][v] = alpha(u, size) * alpha(v, size) * sum
		}
	}

	return output
}

func alpha(index, size int) float64 {
	if index == 0 {
		return math.Sqrt(1.0 / float64(size))
	}
	return math.Sqrt(2.0 / float64(size))
}

func IsAnimatedWebP(data []byte) bool {
	if len(data) < 16 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return false
	}

	offset := 12
	for offset+8 <= len(data) {
		chunkType := string(data[offset : offset+4])
		chunkSize := int(uint32(data[offset+4]) | uint32(data[offset+5])<<8 | uint32(data[offset+6])<<16 | uint32(data[offset+7])<<24)
		dataStart := offset + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > len(data) {
			return false
		}

		if chunkType == "VP8X" && chunkSize >= 1 {
			return data[dataStart]&0x02 != 0
		}

		offset = dataEnd
		if chunkSize%2 == 1 {
			offset++
		}
	}

	return false
}
