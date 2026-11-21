package imageasset

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png" // Register PNG for image.Decode.
)

const (
	MaxSourceBytes  = 10 * 1024 * 1024
	MaxPreviewBytes = 256 * 1024
	PreviewMaxSide  = 1024
	maxSide         = 16384
	maxPixels       = 16 * 1024 * 1024
)

var (
	ErrUnsupported = errors.New("unsupported or unreadable image; use a complete JPEG or PNG")
	ErrTooLarge    = errors.New("image exceeds the size or resolution limit")
)

// Decode checks both the encoded size and dimensions before allocating pixels.
func Decode(data []byte) (image.Image, string, error) {
	if len(data) > MaxSourceBytes {
		return nil, "", ErrTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, "", ErrUnsupported
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width > maxPixels/cfg.Height {
		return nil, "", ErrTooLarge
	}
	decoded, actual, err := image.Decode(bytes.NewReader(data))
	if err != nil || actual != format || decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
		return nil, "", ErrUnsupported
	}
	return decoded, "image/" + format, nil
}

// Preview encodes an opaque JPEG with bounded dimensions and wire size.
func Preview(ctx context.Context, source image.Image) ([]byte, int, int, error) {
	for side := PreviewMaxSide; side >= 128; side /= 2 {
		preview, err := resizePreview(ctx, source, side)
		if err != nil {
			return nil, 0, 0, err
		}
		for _, quality := range []int{82, 65, 50} {
			var encoded bytes.Buffer
			if err := jpeg.Encode(&encoded, preview, &jpeg.Options{Quality: quality}); err != nil {
				return nil, 0, 0, err
			}
			if encoded.Len() <= MaxPreviewBytes {
				return encoded.Bytes(), preview.Bounds().Dx(), preview.Bounds().Dy(), nil
			}
		}
	}
	return nil, 0, 0, ErrTooLarge
}

// Average each source rectangle so thin features remain visible when reduced.
// Composite transparent PNG pixels onto white before encoding JPEG.
func resizePreview(ctx context.Context, source image.Image, maxEdge int) (*image.RGBA, error) {
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return nil, ErrUnsupported
	}
	scale := max(w, h)
	targetW, targetH := w, h
	if scale > maxEdge {
		targetW, targetH = max(1, w*maxEdge/scale), max(1, h*maxEdge/scale)
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	for y := 0; y < targetH; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		y0, y1 := y*h/targetH, (y+1)*h/targetH
		for x := 0; x < targetW; x++ {
			x0, x1 := x*w/targetW, (x+1)*w/targetW
			var red, green, blue uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, a := source.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					red += uint64(r + 0xffff - a)
					green += uint64(g + 0xffff - a)
					blue += uint64(b + 0xffff - a)
				}
			}
			count := uint64((x1 - x0) * (y1 - y0))
			i := dst.PixOffset(x, y)
			dst.Pix[i] = uint8(red / count >> 8)
			dst.Pix[i+1] = uint8(green / count >> 8)
			dst.Pix[i+2] = uint8(blue / count >> 8)
			dst.Pix[i+3] = 255
		}
	}
	return dst, nil
}
