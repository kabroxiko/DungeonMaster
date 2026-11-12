package gamesession

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
)

// previewMaxDimension and previewJPEGQuality are the size of the copy the map modal paints first.
//
// A rendered map is 4096x2304 and about 2.7 MB, which the browser can only show once every byte has arrived. At
// 2048 pixels the same picture is a fifth of the bytes and covers the fitted view on a high-density screen, so it is
// what a client gets unless it explicitly asks for the render to zoom into.
const (
	previewMaxDimension = 2048
	previewJPEGQuality  = 78
)

// downscaledJPEG re-encodes a JPEG so neither side exceeds maxDimension, averaging every source pixel a destination
// pixel covers.
//
// Averaging rather than point sampling is what keeps thin coastline strokes and text from dropping out, which is most
// of what a map is made of. A picture already within the limit is returned unchanged, so callers never pay for a
// re-encode that would only lose quality.
func downscaledJPEG(data []byte, maxDimension, quality int) ([]byte, error) {
	if maxDimension <= 0 {
		return data, nil
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode map image: %w", err)
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("decode map image: empty picture")
	}
	if width <= maxDimension && height <= maxDimension {
		return data, nil
	}

	scale := math.Min(float64(maxDimension)/float64(width), float64(maxDimension)/float64(height))
	targetWidth := int(math.Round(float64(width) * scale))
	targetHeight := int(math.Round(float64(height) * scale))
	if targetWidth < 1 || targetHeight < 1 {
		return nil, fmt.Errorf("decode map image: %dx%d is too small to scale", width, height)
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		y0 := bounds.Min.Y + y*height/targetHeight
		y1 := bounds.Min.Y + (y+1)*height/targetHeight
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < targetWidth; x++ {
			x0 := bounds.Min.X + x*width/targetWidth
			x1 := bounds.Min.X + (x+1)*width/targetWidth
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var red, green, blue, count uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, _ := src.At(sx, sy).RGBA()
					red += uint64(r)
					green += uint64(g)
					blue += uint64(b)
					count++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8((red / count) >> 8),
				G: uint8((green / count) >> 8),
				B: uint8((blue / count) >> 8),
				A: 0xff,
			})
		}
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode map preview: %w", err)
	}
	return out.Bytes(), nil
}
