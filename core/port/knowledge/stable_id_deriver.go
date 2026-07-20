package knowledge

import domainknowledge "myai/core/domain/knowledge"

type StableIDDeriver interface {
	ChunkID(identity domainknowledge.ChunkIdentity) string
	EmbeddingID(chunkID string, embeddingProfileID string) string
}
