package imageasset

import "encoding/binary"

func inspectWebP(data []byte) (Info, error) {
	invalid := func() (Info, error) { return Info{}, ErrUnsupported }
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return invalid()
	}
	riffEnd := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if riffEnd != uint64(len(data)) || riffEnd < 12 {
		return invalid()
	}
	info := Info{MIMEType: "image/webp", FrameCount: 1}
	frameCount := 0
	var canvasSet, extended bool
	var hasVP8, hasVP8L, animated bool
	var totalFramePixels uint64
	offset := 12
	for offset+8 <= len(data) {
		name := string(data[offset : offset+4])
		size := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		start := uint64(offset + 8)
		end := start + size
		if end > riffEnd || end < start {
			return invalid()
		}
		chunk := data[start:end]
		switch name {
		case "VP8X":
			if extended || len(chunk) != 10 {
				return invalid()
			}
			extended = true
			flags := chunk[0]
			if flags&0xc1 != 0 {
				return invalid()
			}
			info.HasAlpha = flags&0x10 != 0
			animated = flags&0x02 != 0
			info.Width = read24(chunk[4:7]) + 1
			info.Height = read24(chunk[7:10]) + 1
			canvasSet = true
		case "VP8 ":
			if len(chunk) < 10 || chunk[3] != 0x9d || chunk[4] != 0x01 || chunk[5] != 0x2a {
				return invalid()
			}
			width := int(binary.LittleEndian.Uint16(chunk[6:8]) & 0x3fff)
			height := int(binary.LittleEndian.Uint16(chunk[8:10]) & 0x3fff)
			if !canvasSet {
				info.Width, info.Height, canvasSet = width, height, true
			}
			hasVP8 = true
		case "VP8L":
			if len(chunk) < 5 || chunk[0] != 0x2f {
				return invalid()
			}
			width := 1 + int(chunk[1]) + int(chunk[2]&0x3f)<<8
			height := 1 + int(chunk[2]>>6) + int(chunk[3])<<2 + int(chunk[4]&0x0f)<<10
			if !canvasSet {
				info.Width, info.Height, canvasSet = width, height, true
			}
			info.HasAlpha = info.HasAlpha || chunk[4]&0x10 != 0
			hasVP8L = true
		case "ALPH":
			info.HasAlpha = true
		case "ANIM":
			if len(chunk) != 6 {
				return invalid()
			}
			animated = true
		case "ANMF":
			if len(chunk) < 16 || !canvasSet {
				return invalid()
			}
			frameX := read24(chunk[0:3]) * 2
			frameY := read24(chunk[3:6]) * 2
			frameW := read24(chunk[6:9]) + 1
			frameH := read24(chunk[9:12]) + 1
			if frameX+frameW > info.Width || frameY+frameH > info.Height {
				return invalid()
			}
			pixels := uint64(frameW) * uint64(frameH)
			if pixels > maxAnimatedPixels-totalFramePixels {
				return Info{}, ErrTooLarge
			}
			totalFramePixels += pixels
			frameCount++
			animated = true
		}
		offset = int(end)
		if size&1 != 0 {
			offset++
			if uint64(offset) > riffEnd {
				return invalid()
			}
		}
	}
	if offset != len(data) {
		return invalid()
	}
	if !canvasSet || info.Width < 1 || info.Height < 1 {
		return invalid()
	}
	if err := validateDimensions(info.Width, info.Height); err != nil {
		return Info{}, err
	}
	if animated {
		if frameCount == 0 {
			return invalid()
		}
		info.FrameCount = frameCount
		info.IsAnimated = true
	} else if !hasVP8 && !hasVP8L {
		return invalid()
	}
	if !animated {
		info.FrameCount = 1
	}
	if info.FrameCount > 1 && totalFramePixels > maxAnimatedPixels {
		return Info{}, ErrTooLarge
	}
	return info, nil
}

func read24(data []byte) int { return int(data[0]) | int(data[1])<<8 | int(data[2])<<16 }
