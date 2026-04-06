package imagehash

import (
	"errors"
	"image"
	"image/color"

	"golang.org/x/image/draw"
)

func ComputeVector(img image.Image) ([]float32, error) {
	if img == nil {
		return nil, errors.New("nil image")
	}

	gray := resizeToGray(img, 16, 16)
	vector := make([]float32, 0, 256)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			vector = append(vector, float32(gray.GrayAt(x, y).Y)/255.0)
		}
	}
	return vector, nil
}

func resizeToGray(src image.Image, width, height int) *image.Gray {
	dst := image.NewGray(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), toNRGBA(src), src.Bounds(), draw.Over, nil)
	return dst
}

func toNRGBA(src image.Image) *image.NRGBA {
	bounds := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(x-bounds.Min.X, y-bounds.Min.Y, color.NRGBAModel.Convert(src.At(x, y)))
		}
	}
	return dst
}
