package gamesession

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// encodeTestJPEG builds a picture whose left half is dark and right half is light, so a scale that dropped detail would
// show up as a uniform strip where the boundary should be.
func encodeTestJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, width, height))
	dark := color.RGBA{R: 20, G: 30, B: 40, A: 0xff}
	light := color.RGBA{R: 230, G: 220, B: 200, A: 0xff}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if x < width/2 {
				src.SetRGBA(x, y, dark)
			} else {
				src.SetRGBA(x, y, light)
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode test picture: %v", err)
	}
	return buf.Bytes()
}

func TestDownscaledJPEGFitsThePreviewLimitAndKeepsTheAspectRatio(t *testing.T) {
	full := encodeTestJPEG(t, 4096, 2304)

	preview, err := downscaledJPEG(full, previewMaxDimension, previewJPEGQuality)
	if err != nil {
		t.Fatalf("downscale: %v", err)
	}
	if len(preview) >= len(full) {
		t.Fatalf("preview is %d bytes, full picture is %d: no saving", len(preview), len(full))
	}
	decoded, _, err := image.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() > previewMaxDimension || bounds.Dy() > previewMaxDimension {
		t.Fatalf("preview is %dx%d, which exceeds the %d limit", bounds.Dx(), bounds.Dy(), previewMaxDimension)
	}
	if bounds.Dx() != previewMaxDimension {
		t.Fatalf("preview width is %d, want %d", bounds.Dx(), previewMaxDimension)
	}
	if bounds.Dy() != previewMaxDimension*1152/2048 {
		t.Fatalf("preview height is %d, want the source aspect ratio kept", bounds.Dy())
	}

	// Both halves must survive: a preview that lost the coastline is not a preview of a map.
	left := decoded.At(4, bounds.Dy()/2)
	right := decoded.At(bounds.Dx()-5, bounds.Dy()/2)
	leftGrey := greyOf(left)
	rightGrey := greyOf(right)
	if rightGrey-leftGrey < 100 {
		t.Fatalf("preview lost its contrast: left=%d right=%d", leftGrey, rightGrey)
	}
}

func TestDownscaledJPEGLeavesAPictureWithinTheLimitAlone(t *testing.T) {
	small := encodeTestJPEG(t, 640, 360)

	preview, err := downscaledJPEG(small, previewMaxDimension, previewJPEGQuality)
	if err != nil {
		t.Fatalf("downscale: %v", err)
	}
	if !bytes.Equal(preview, small) {
		t.Fatal("a picture already within the limit was re-encoded and lost quality")
	}
}

func TestDownscaledJPEGRejectsBytesThatAreNotAPicture(t *testing.T) {
	if _, err := downscaledJPEG([]byte("not a jpeg"), previewMaxDimension, previewJPEGQuality); err == nil {
		t.Fatal("expected an error for bytes that are not a picture")
	}
}

func greyOf(c color.Color) int {
	r, g, b, _ := c.RGBA()
	return int((r + g + b) / 3 >> 8)
}
