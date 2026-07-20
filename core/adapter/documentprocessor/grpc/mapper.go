package grpcprocessor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	pb "myai/core/adapter/documentprocessor/grpc/pb"
	domainknowledge "myai/core/domain/knowledge"
)

func parsingProfileMessage(profile domainknowledge.ParsingProfile) *pb.ParsingProfile {
	return &pb.ParsingProfile{
		Id:            profile.ID,
		Name:          profile.Name,
		ParserId:      profile.ParserID,
		ParserVersion: profile.ParserVersion,
		Ocr:           profile.OCR,
		Options:       cloneStringMap(profile.Options),
	}
}

func chunkingProfileMessage(profile domainknowledge.ChunkingProfile) *pb.ChunkingProfile {
	return &pb.ChunkingProfile{
		Id:              profile.ID,
		Name:            profile.Name,
		StrategyId:      profile.StrategyID,
		StrategyVersion: profile.StrategyVersion,
		MaxChunkSize:    int32(profile.MaxChunkSize),
		Overlap:         int32(profile.Overlap),
		Tokenizer:       profile.Tokenizer,
		Options:         cloneStringMap(profile.Options),
	}
}

func chunkDraftDomain(value *pb.ChunkDraft) (domainknowledge.ChunkDraft, error) {
	if value.GetStartOffset() > math.MaxInt || value.GetEndOffset() > math.MaxInt {
		return domainknowledge.ChunkDraft{}, fmt.Errorf("chunk offsets exceed platform integer range")
	}
	draft := domainknowledge.ChunkDraft{
		Ordinal:       int(value.GetOrdinal()),
		Text:          value.GetText(),
		ContentHash:   contentHash(value.GetText()),
		StartOffset:   int(value.GetStartOffset()),
		EndOffset:     int(value.GetEndOffset()),
		SourcePage:    int(value.GetSourcePage()),
		SourceHeading: value.GetSourceHeading(),
	}
	if err := draft.Validate(); err != nil {
		return domainknowledge.ChunkDraft{}, err
	}
	return draft, nil
}

func contentHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
