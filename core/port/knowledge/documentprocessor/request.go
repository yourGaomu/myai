package documentprocessor

import (
	"io"

	domainknowledge "myai/core/domain/knowledge"
)

type Request struct {
	Document        domainknowledge.Document
	ParsingProfile  domainknowledge.ParsingProfile
	ChunkingProfile domainknowledge.ChunkingProfile
	Content         io.Reader
}
