package protocol

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"
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
