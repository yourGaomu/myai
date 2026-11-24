package imageasset

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"

	_ "golang.org/x/image/webp"
)

const (
	MaxSourceBytes       = 16 * 1024 * 1024
	MaxPreviewBytes      = 2 * 1024 * 1024
	PreviewMaxEdge       = 2048
	PreviewOpaqueMaxEdge = 1024
	maxSide              = 16384
	maxPixels            = 16 * 1024 * 1024
	maxAnimatedPixels    = 128 * 1024 * 1024
)

var (
	ErrUnsupported = errors.New("unsupported or unreadable image; use a complete JPEG, PNG, GIF, or WebP")
	ErrTooLarge    = errors.New("image exceeds the size or resolution limit")
)

type Info struct {
	MIMEType   string
	Width      int
	Height     int
	HasAlpha   bool
	IsAnimated bool
	FrameCount int
}

// Inspect validates an image container and its dimensions without decoding all
// pixels. Animated GIF frames are preflighted before DecodeAll can allocate.
func Inspect(data []byte) (Info, error) {
	if len(data) > MaxSourceBytes {
		return Info{}, ErrTooLarge
	}
	if len(data) == 0 {
		return Info{}, ErrUnsupported
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
			return inspectWebP(data)
		}
		return Info{}, ErrUnsupported
	}
	if err := validateDimensions(config.Width, config.Height); err != nil {
		return Info{}, err
	}
	info := Info{Width: config.Width, Height: config.Height, FrameCount: 1}
	switch format {
	case "jpeg":
		info.MIMEType = "image/jpeg"
	case "png":
		info.MIMEType = "image/png"
		alpha, err := pngHasAlpha(data)
		if err != nil {
			return Info{}, ErrUnsupported
		}
		info.HasAlpha = alpha
	case "gif":
		info.MIMEType = "image/gif"
		gifInfo, err := inspectGIF(data)
		if err != nil {
			return Info{}, err
		}
		if gifInfo.Width != config.Width || gifInfo.Height != config.Height {
			return Info{}, ErrUnsupported
		}
		info.FrameCount, info.HasAlpha = gifInfo.FrameCount, gifInfo.HasAlpha
		info.IsAnimated = gifInfo.FrameCount > 1
	case "webp":
		webpInfo, err := inspectWebP(data)
		if err != nil {
			return Info{}, err
		}
		if webpInfo.Width != config.Width || webpInfo.Height != config.Height {
			return Info{}, ErrUnsupported
		}
		return webpInfo, nil
	default:
		return Info{}, ErrUnsupported
	}
	if info.MIMEType == "image/gif" {
		all, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil || len(all.Image) != info.FrameCount || all.Config.Width != info.Width || all.Config.Height != info.Height {
			return Info{}, ErrUnsupported
		}
	} else if !info.IsAnimated {
		decoded, actual, err := image.Decode(bytes.NewReader(data))
		if err != nil || formatMIME(actual) != info.MIMEType || decoded.Bounds().Dx() != info.Width || decoded.Bounds().Dy() != info.Height {
			return Info{}, ErrUnsupported
		}
	}
	return info, nil
}

// Decode validates and decodes the first frame for consumers such as vision
// models that accept a single image. PreviewBytes should be used to preserve
// animated images for display.
func Decode(data []byte) (image.Image, string, error) {
	info, err := Inspect(data)
	if err != nil {
		return nil, "", err
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || decoded.Bounds().Dx() != info.Width || decoded.Bounds().Dy() != info.Height {
		return nil, "", ErrUnsupported
	}
	return decoded, formatMIME(format), nil
}

// PreviewBytes returns a size-limited, format-aware preview. Small animations
// are passed through unchanged; transparency is retained for PNG previews.
func PreviewBytes(ctx context.Context, data []byte) ([]byte, string, int, int, error) {
	info, err := Inspect(data)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if info.IsAnimated {
		if len(data) <= MaxPreviewBytes && max(info.Width, info.Height) <= PreviewMaxEdge {
			return append([]byte(nil), data...), info.MIMEType, info.Width, info.Height, nil
		}
		if info.MIMEType == "image/webp" {
			return nil, "", 0, 0, errors.New("animated WebP exceeds preview limits; open the original image")
		}
		preview, err := previewGIF(ctx, data, info)
		if err != nil {
			return nil, "", 0, 0, err
		}
		return preview, "image/gif", scaledDimension(info.Width, max(info.Width, info.Height), PreviewMaxEdge),
			scaledDimension(info.Height, max(info.Width, info.Height), PreviewMaxEdge), nil
	}

	// Preserve source bytes and alpha whenever a transparent still already fits.
	if info.HasAlpha && (info.MIMEType == "image/png" || info.MIMEType == "image/webp") &&
		len(data) <= MaxPreviewBytes && max(info.Width, info.Height) <= PreviewMaxEdge {
		return append([]byte(nil), data...), info.MIMEType, info.Width, info.Height, nil
	}

	decoded, _, err := Decode(data)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if info.HasAlpha {
		for edge := PreviewMaxEdge; edge >= 128; edge = edge * 3 / 4 {
			resized, err := resizeRGBA(ctx, decoded, edge, true)
			if err != nil {
				return nil, "", 0, 0, err
			}
			var out bytes.Buffer
			if err := png.Encode(&out, resized); err != nil {
				return nil, "", 0, 0, err
			}
			if out.Len() <= MaxPreviewBytes {
				return out.Bytes(), "image/png", resized.Bounds().Dx(), resized.Bounds().Dy(), nil
			}
		}
		return nil, "", 0, 0, ErrTooLarge
	}
	for edge := PreviewOpaqueMaxEdge; edge >= 128; edge /= 2 {
		resized, err := resizeRGBA(ctx, decoded, edge, false)
		if err != nil {
			return nil, "", 0, 0, err
		}
		for _, quality := range []int{82, 70, 55} {
			var out bytes.Buffer
			if err := jpeg.Encode(&out, resized, &jpeg.Options{Quality: quality}); err != nil {
				return nil, "", 0, 0, err
			}
			if out.Len() <= MaxPreviewBytes {
				return out.Bytes(), "image/jpeg", resized.Bounds().Dx(), resized.Bounds().Dy(), nil
			}
		}
	}
	return nil, "", 0, 0, ErrTooLarge
}

// Preview retains the legacy opaque JPEG helper for callers that already have
// decoded pixels. New protocol code should use PreviewBytes.
func Preview(ctx context.Context, source image.Image) ([]byte, int, int, error) {
	for side := PreviewOpaqueMaxEdge; side >= 128; side /= 2 {
		preview, err := resizeRGBA(ctx, source, side, false)
		if err != nil {
			return nil, 0, 0, err
		}
		for _, quality := range []int{82, 70, 55} {
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

func resizeRGBA(ctx context.Context, source image.Image, maxEdge int, preserveAlpha bool) (*image.RGBA, error) {
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
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
			var red, green, blue, alpha uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, a := source.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					red += uint64(r)
					green += uint64(g)
					blue += uint64(b)
					alpha += uint64(a)
				}
			}
			count := uint64((x1 - x0) * (y1 - y0))
			if !preserveAlpha {
				red += count*0xffff - alpha
				green += count*0xffff - alpha
				blue += count*0xffff - alpha
				alpha = count * 0xffff
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i] = uint8(red / count >> 8)
			dst.Pix[i+1] = uint8(green / count >> 8)
			dst.Pix[i+2] = uint8(blue / count >> 8)
			dst.Pix[i+3] = uint8(alpha / count >> 8)
		}
	}
	return dst, nil
}

func previewGIF(ctx context.Context, data []byte, info Info) ([]byte, error) {
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil || len(decoded.Image) != info.FrameCount {
		return nil, ErrUnsupported
	}
	for edge := PreviewMaxEdge; edge >= 128; edge = edge * 3 / 4 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		scaleNum, scaleDen := min(edge, max(info.Width, info.Height)), max(info.Width, info.Height)
		width, height := max(1, info.Width*scaleNum/scaleDen), max(1, info.Height*scaleNum/scaleDen)
		resized := &gif.GIF{
			Config:    image.Config{Width: width, Height: height, ColorModel: decoded.Config.ColorModel},
			LoopCount: decoded.LoopCount, Delay: append([]int(nil), decoded.Delay...),
			Disposal: append([]byte(nil), decoded.Disposal...), BackgroundIndex: decoded.BackgroundIndex,
		}
		for _, frame := range decoded.Image {
			bounds := frame.Bounds()
			x0, y0 := bounds.Min.X*width/info.Width, bounds.Min.Y*height/info.Height
			x1, y1 := bounds.Max.X*width/info.Width, bounds.Max.Y*height/info.Height
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			if x1 > width {
				x1 = width
			}
			if y1 > height {
				y1 = height
			}
			dst := image.NewPaletted(image.Rect(x0, y0, x1, y1), frame.Palette)
			for y := y0; y < y1; y++ {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				sy := bounds.Min.Y + (y-y0)*bounds.Dy()/(y1-y0)
				for x := x0; x < x1; x++ {
					sx := bounds.Min.X + (x-x0)*bounds.Dx()/(x1-x0)
					dst.SetColorIndex(x, y, frame.ColorIndexAt(sx, sy))
				}
			}
			resized.Image = append(resized.Image, dst)
		}
		var out bytes.Buffer
		if err := gif.EncodeAll(&out, resized); err != nil {
			return nil, err
		}
		if out.Len() <= MaxPreviewBytes {
			return out.Bytes(), nil
		}
	}
	return nil, ErrTooLarge
}

func scaledDimension(value, sourceMax, targetMax int) int {
	if sourceMax <= targetMax {
		return value
	}
	return max(1, value*targetMax/sourceMax)
}

func validateDimensions(width, height int) error {
	if width < 1 || height < 1 || width > maxSide || height > maxSide || width > maxPixels/height {
		return ErrTooLarge
	}
	return nil
}

func formatMIME(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func pngHasAlpha(data []byte) (bool, error) {
	if len(data) < 8 || !bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return false, ErrUnsupported
	}
	for offset := 8; offset+12 <= len(data); {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if length < 0 || length > len(data)-offset-12 {
			return false, ErrUnsupported
		}
		kind := string(data[offset+4 : offset+8])
		if kind == "IHDR" {
			if length != 13 {
				return false, ErrUnsupported
			}
			colorType := data[offset+8+9]
			if colorType == 4 || colorType == 6 {
				return true, nil
			}
		}
		if kind == "tRNS" {
			return true, nil
		}
		if kind == "IEND" {
			return false, nil
		}
		offset += 12 + length
	}
	return false, io.ErrUnexpectedEOF
}
