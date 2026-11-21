package model

import (
	"fmt"

	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
)

const (
	MaxInputImages          = 4
	MaxInputImageBytes      = 5 * 1024 * 1024
	MaxTotalInputImageBytes = 12 * 1024 * 1024
)

// InputImage is verified JPEG or PNG data for a user message in this request.
// Its private byte field keeps it out of accidental JSON serialization.
type InputImage struct {
	MessageIndex int
	MIMEType     string
	data         []byte
}

// NewInputImage accepts verified JPEG or PNG bytes. Callers and model adapters
// treat the byte slice as read-only so a turn can reuse it across samples.
func NewInputImage(mimeType string, data []byte) InputImage {
	return InputImage{MIMEType: mimeType, data: data}
}

func (image InputImage) Bytes() []byte { return image.data }

// Format keeps data out of accidental request diagnostics, including %#v.
func (image InputImage) Format(state fmt.State, _ rune) {
	fmt.Fprintf(state, "InputImage{MessageIndex:%d MIMEType:%q Bytes:%d}", image.MessageIndex, image.MIMEType, len(image.data))
}

type GenerateRequest struct {
	Messages []domainmessage.Message
	Images   []InputImage
	Tools    []Tool
	Stream   ChatStreamHandler
	Settings generation.ResolvedSettings
}
