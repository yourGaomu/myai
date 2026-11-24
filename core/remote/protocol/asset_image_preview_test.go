package protocol

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"myai/core/imageasset"
)

func TestAssetImagePreviewRejectsInvalidIdentity(t *testing.T) {
	for _, tc := range []struct {
		requestID string
		sessionID string
		assetID   string
	}{
		{"", "session", "ast_image"},
		{"preview", "", "ast_image"},
		{"preview", " session", "ast_image"},
		{"preview", "session", "../image"},
	} {
		message, err := NewMessage(TypeAssetImagePreview, tc.requestID, "", "", tc.sessionID, AssetImagePreviewPayload{AssetID: tc.assetID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := AssetImagePreviewRequest(message); err == nil {
			t.Fatalf("accepted invalid preview identity: %#v", tc)
		}
	}
}

func TestAssetImagePreviewChecksEncodedImageDimensions(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewGray(image.Rect(0, 0, 10, 20)), nil); err != nil {
		t.Fatal(err)
	}
	preview := AssetImagePreviewResultPayload{
		AssetID: "ast_image", ContentType: "image/jpeg", Data: encoded.Bytes(), Width: 10, Height: 20,
	}
	check := func(payload AssetImagePreviewResultPayload) error {
		t.Helper()
		message, err := NewMessage(TypeAssetImagePreviewResult, "preview", "user", "device", "session", payload)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := AssetImagePreviewResult(message)
		if err == nil && !bytes.Equal(parsed.Data, payload.Data) {
			t.Fatal("preview response changed the JPEG bytes")
		}
		return err
	}
	if err := check(preview); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*AssetImagePreviewResultPayload){
		func(p *AssetImagePreviewResultPayload) { p.ContentType = "image/png" },
		func(p *AssetImagePreviewResultPayload) { p.Height = 19 },
		func(p *AssetImagePreviewResultPayload) { p.Width = AssetImagePreviewMaxDimension + 1 },
		func(p *AssetImagePreviewResultPayload) { p.Data = []byte{0xff, 0xd8, 0xff} },
		func(p *AssetImagePreviewResultPayload) {
			p.Data = append(p.Data[:0:0], make([]byte, AssetImagePreviewMaxBytes+1)...)
		},
	} {
		invalid := preview
		change(&invalid)
		if err := check(invalid); err == nil {
			t.Fatal("accepted a preview outside the image contract")
		}
	}
}

func TestAssetImagePreviewAcceptsAllSupportedFormats(t *testing.T) {
	palette := color.Palette{color.RGBA{A: 0}, color.RGBA{R: 255, A: 255}}
	frame := func(index uint8) *image.Paletted {
		img := image.NewPaletted(image.Rect(0, 0, 12, 9), palette)
		for y := 0; y < 9; y++ {
			for x := 0; x < 12; x++ {
				img.SetColorIndex(x, y, index)
			}
		}
		return img
	}
	var gifBytes bytes.Buffer
	if err := gif.EncodeAll(&gifBytes, &gif.GIF{Image: []*image.Paletted{frame(0), frame(1)}, Delay: []int{5, 7}, LoopCount: 0}); err != nil {
		t.Fatal(err)
	}
	webpBytes, err := base64.StdEncoding.DecodeString("UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA==")
	if err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		name, mime string
		data       []byte
	}{
		{"jpeg", "image/jpeg", encodedPreview(t, "jpeg")},
		{"png", "image/png", encodedPreview(t, "png")},
		{"gif", "image/gif", gifBytes.Bytes()},
		{"webp", "image/webp", webpBytes},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			info, err := imageasset.Inspect(fixture.data)
			if err != nil {
				t.Fatal(err)
			}
			payload := AssetImagePreviewResultPayload{AssetID: "ast_image", ContentType: fixture.mime, Data: fixture.data, Width: info.Width, Height: info.Height}
			message, err := NewMessage(TypeAssetImagePreviewResult, "preview", "user", "device", "session", payload)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := AssetImagePreviewResult(message)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.ContentType != fixture.mime || !bytes.Equal(parsed.Data, fixture.data) {
				t.Fatalf("format changed: %+v", parsed)
			}
			payload.ContentType = "image/png"
			if fixture.mime == "image/png" {
				payload.ContentType = "image/jpeg"
			}
			message, err = NewMessage(TypeAssetImagePreviewResult, "preview", "user", "device", "session", payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := AssetImagePreviewResult(message); err == nil {
				t.Fatal("accepted MIME that did not match the bytes")
			}
		})
	}
}

func encodedPreview(t *testing.T, format string) []byte {
	t.Helper()
	var encoded bytes.Buffer
	pixels := image.NewNRGBA(image.Rect(0, 0, 12, 9))
	pixels.SetNRGBA(1, 2, color.NRGBA{R: 240, G: 20, B: 10, A: 180})
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&encoded, pixels, nil)
	case "png":
		err = png.Encode(&encoded, pixels)
	}
	if err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
