package imageasset

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/gif"
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

func TestInspectAndPreviewPreserveTransparentPNG(t *testing.T) {
	transparent := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	transparent.SetNRGBA(0, 0, color.NRGBA{R: 240, G: 20, B: 10, A: 255})
	transparent.SetNRGBA(1, 0, color.NRGBA{R: 20, G: 240, B: 10, A: 96})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, transparent); err != nil {
		t.Fatal(err)
	}
	info, err := Inspect(encoded.Bytes())
	if err != nil || info.MIMEType != "image/png" || !info.HasAlpha || info.Width != 3 || info.Height != 2 {
		t.Fatalf("Inspect=%+v, err=%v", info, err)
	}
	preview, mime, width, height, err := PreviewBytes(t.Context(), encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" || width != 3 || height != 2 || !bytes.Equal(preview, encoded.Bytes()) {
		t.Fatalf("small transparent image was not passed through: %s %dx%d", mime, width, height)
	}
	decoded, err := png.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := decoded.At(1, 0).RGBA()
	if alpha == 0xffff {
		t.Fatal("transparency was flattened")
	}
}

func TestPreviewResizesTransparentPNGWithoutWhiteMatte(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2400, 1200))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			if x < 1200 {
				source.SetNRGBA(x, y, color.NRGBA{R: 220, G: 30, B: 15, A: 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	preview, mime, width, height, err := PreviewBytes(t.Context(), encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" || width > PreviewMaxEdge || height > PreviewMaxEdge || len(preview) > MaxPreviewBytes {
		t.Fatalf("preview contract violated: mime=%s %dx%d bytes=%d", mime, width, height, len(preview))
	}
	decoded, err := png.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := decoded.At(width-1, height/2).RGBA()
	if alpha != 0 {
		t.Fatalf("transparent region acquired alpha=%d", alpha)
	}
}

func TestAnimatedGIFPassThroughAndResize(t *testing.T) {
	palette := color.Palette{color.RGBA{A: 0}, color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	frame := func(width, height int, seed uint32) *image.Paletted {
		img := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				seed = seed*1664525 + 1013904223
				img.SetColorIndex(x, y, uint8(seed>>24)%4)
			}
		}
		return img
	}
	small := &gif.GIF{Image: []*image.Paletted{frame(10, 8, 1), frame(10, 8, 2)}, Delay: []int{3, 7}, LoopCount: 0, Disposal: []byte{gif.DisposalBackground, gif.DisposalPrevious}}
	var smallBytes bytes.Buffer
	if err := gif.EncodeAll(&smallBytes, small); err != nil {
		t.Fatal(err)
	}
	info, err := Inspect(smallBytes.Bytes())
	if err != nil || !info.IsAnimated || info.FrameCount != 2 || !info.HasAlpha {
		t.Fatalf("GIF info=%+v err=%v", info, err)
	}
	preview, mime, width, height, err := PreviewBytes(t.Context(), smallBytes.Bytes())
	if err != nil || mime != "image/gif" || width != 10 || height != 8 || !bytes.Equal(preview, smallBytes.Bytes()) {
		t.Fatalf("small GIF not passed through: %s %dx%d err=%v", mime, width, height, err)
	}

	large := &gif.GIF{Image: []*image.Paletted{frame(1600, 1200, 3), frame(1600, 1200, 4)}, Delay: []int{4, 9}, LoopCount: 2, Disposal: []byte{gif.DisposalBackground, gif.DisposalPrevious}}
	var largeBytes bytes.Buffer
	if err := gif.EncodeAll(&largeBytes, large); err != nil {
		t.Fatal(err)
	}
	if largeBytes.Len() <= MaxPreviewBytes {
		t.Skipf("GIF fixture was unexpectedly compressible: %d bytes", largeBytes.Len())
	}
	preview, mime, width, height, err = PreviewBytes(t.Context(), largeBytes.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/gif" || width > PreviewMaxEdge || height > PreviewMaxEdge || len(preview) > MaxPreviewBytes {
		t.Fatalf("large GIF preview contract: %s %dx%d %d bytes", mime, width, height, len(preview))
	}
	resized, err := gif.DecodeAll(bytes.NewReader(preview))
	if err != nil || len(resized.Image) != 2 || resized.LoopCount != 2 || resized.Delay[0] != 4 || resized.Delay[1] != 9 || resized.Disposal[1] != gif.DisposalPrevious {
		t.Fatalf("GIF timing/disposal lost: %+v err=%v", resized, err)
	}
}

func TestWebPInspectAndDecode(t *testing.T) {
	fixture, err := base64.StdEncoding.DecodeString("UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA==")
	if err != nil {
		t.Fatal(err)
	}
	info, err := Inspect(fixture)
	if err != nil || info.MIMEType != "image/webp" || info.Width < 1 || info.Height < 1 {
		t.Fatalf("WebP info=%+v err=%v", info, err)
	}
	decoded, mime, err := Decode(fixture)
	if err != nil || mime != "image/webp" || decoded.Bounds().Dx() != info.Width {
		t.Fatalf("WebP decode: %s %v", mime, err)
	}
}

func TestGIFPreflightStopsBeforeExcessiveFrameDecodeBudget(t *testing.T) {
	data := append([]byte("GIF89a"), 0, 64, 0, 4, 0, 0, 0)
	for range 9 {
		data = append(data, 0x2c, 0, 0, 0, 0, 0, 64, 0, 4, 0, 2, 0)
	}
	data = append(data, 0x3b)
	if _, err := inspectGIF(data); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected preflight budget error, got %v", err)
	}
}
