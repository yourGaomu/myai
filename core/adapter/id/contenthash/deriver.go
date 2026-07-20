package contenthash

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"strconv"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type Deriver struct{}

var _ knowledgeport.StableIDDeriver = Deriver{}

func (Deriver) ChunkID(identity domainknowledge.ChunkIdentity) string {
	hasher := sha256.New()
	writePart(hasher, identity.DocumentID)
	writePart(hasher, strconv.FormatInt(identity.DocumentVersion, 10))
	writePart(hasher, identity.ParsingProfileID)
	writePart(hasher, identity.ChunkingProfileID)
	writePart(hasher, strconv.Itoa(identity.Ordinal))
	writePart(hasher, identity.ContentHash)
	return "chk_" + hex.EncodeToString(hasher.Sum(nil))
}

func (Deriver) EmbeddingID(chunkID string, embeddingProfileID string) string {
	hasher := sha256.New()
	writePart(hasher, chunkID)
	writePart(hasher, embeddingProfileID)
	return "emb_" + hex.EncodeToString(hasher.Sum(nil))
}

func writePart(target hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = target.Write(length[:])
	_, _ = target.Write([]byte(value))
}
