package imageasset

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestPreviewPreservesImageWithinWireLimits(t *testing.T) {
	original := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 960; x++ {
			original.SetRGBA(x, y, color.RGBA{R: 200, G: 20, B: 40, A: 255})
		}
	}
	var source bytes.Buffer
	if err := png.Encode(&source, original); err != nil {
		t.Fatal(err)
	}
	decoded, contentType, err := Decode(source.Bytes())
	if err != nil || contentType != "image/png" {
		t.Fatalf("decode PNG: type=%q err=%v", contentType, err)
	}
	data, width, height, err := Preview(t.Context(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	if width != 1024 || height != 576 || len(data) > MaxPreviewBytes {
		t.Fatalf("preview dimensions=%dx%d size=%d", width, height, len(data))
	}
	preview, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, _, _ := preview.At(20, 20).RGBA()
	if r <= 4*g {
		t.Fatal("colored region was lost in the preview")
	}
	r, g, b, _ := preview.At(800, 20).RGBA()
	if r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Fatal("transparent region was not composited on white")
	}
}

func TestDecodeRejectsNonImagesTruncatedAndExcessiveInput(t *testing.T) {
	if _, _, err := Decode([]byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	if _, _, err := Decode(make([]byte, MaxSourceBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewGray(image.Rect(0, 0, 20, 20)), nil); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	if _, _, err := Decode(data[:len(data)/2]); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, _, err := Preview(ctx, image.NewGray(image.Rect(0, 0, 2, 2))); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
