package imageasset

import (
	"encoding/binary"
	"errors"
)

type gifInfo struct {
	Width, Height int
	FrameCount    int
	HasAlpha      bool
}

func inspectGIF(data []byte) (gifInfo, error) {
	var info gifInfo
	if len(data) < 13 || string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a" {
		return info, ErrUnsupported
	}
	info.Width = int(binary.LittleEndian.Uint16(data[6:8]))
	info.Height = int(binary.LittleEndian.Uint16(data[8:10]))
	if err := validateDimensions(info.Width, info.Height); err != nil {
		return info, err
	}
	packed := data[10]
	offset := 13
	if packed&0x80 != 0 {
		offset += 3 * (1 << ((packed & 7) + 1))
	}
	if offset > len(data) {
		return info, ErrUnsupported
	}
	var decodedPixels uint64
	terminated := false
	for offset < len(data) {
		marker := data[offset]
		offset++
		switch marker {
		case 0x3b: // trailer
			terminated = true
			if offset != len(data) || info.FrameCount == 0 {
				return info, ErrUnsupported
			}
			if info.FrameCount > 1 && decodedPixels > maxAnimatedPixels {
				return info, ErrTooLarge
			}
			return info, nil
		case 0x21: // extension
			if offset >= len(data) {
				return info, ErrUnsupported
			}
			label := data[offset]
			offset++
			if label == 0xf9 {
				if offset+6 > len(data) || data[offset] != 4 || data[offset+5] != 0 {
					return info, ErrUnsupported
				}
				if data[offset+1]&1 != 0 {
					info.HasAlpha = true
				}
				offset += 6
				continue
			}
			var err error
			offset, err = skipGIFSubBlocks(data, offset)
			if err != nil {
				return info, err
			}
		case 0x2c: // image descriptor
			if offset+9 > len(data) {
				return info, ErrUnsupported
			}
			left := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			top := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
			width := int(binary.LittleEndian.Uint16(data[offset+4 : offset+6]))
			height := int(binary.LittleEndian.Uint16(data[offset+6 : offset+8]))
			frameFlags := data[offset+8]
			if width < 1 || height < 1 || left+width > info.Width || top+height > info.Height {
				return info, ErrUnsupported
			}
			framePixels := uint64(width) * uint64(height)
			if framePixels > maxAnimatedPixels-decodedPixels {
				return info, ErrTooLarge
			}
			decodedPixels += framePixels
			info.FrameCount++
			offset += 9
			if frameFlags&0x80 != 0 {
				offset += 3 * (1 << ((frameFlags & 7) + 1))
			}
			if offset >= len(data) || data[offset] > 11 {
				return info, ErrUnsupported
			}
			offset++ // LZW minimum code size
			var err error
			offset, err = skipGIFSubBlocks(data, offset)
			if err != nil {
				return info, err
			}
		default:
			return info, ErrUnsupported
		}
		if offset > len(data) {
			return info, ErrUnsupported
		}
	}
	if !terminated {
		return info, errors.New("truncated GIF")
	}
	return info, nil
}

func skipGIFSubBlocks(data []byte, offset int) (int, error) {
	for {
		if offset >= len(data) {
			return 0, ErrUnsupported
		}
		size := int(data[offset])
		offset++
		if size == 0 {
			return offset, nil
		}
		if size > len(data)-offset {
			return 0, ErrUnsupported
		}
		offset += size
	}
}
