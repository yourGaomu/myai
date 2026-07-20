package session

import (
	"fmt"
	"strings"
)

const (
	DefaultRAGTopK = 8
	MaxRAGTopK     = 100
)

type RetrievalMode string

const (
	RetrievalModeOff    RetrievalMode = "off"
	RetrievalModeManual RetrievalMode = "manual"
	RetrievalModeAuto   RetrievalMode = "auto"
	RetrievalModeAlways RetrievalMode = "always"
)

type RAGSettings struct {
	Mode             RetrievalMode
	KnowledgeBaseIDs []string
	CategoryIDs      []string
	TopK             int
}

func DefaultRAGSettings() RAGSettings {
	return RAGSettings{Mode: RetrievalModeAuto, TopK: DefaultRAGTopK}
}

func NormalizeRAGSettings(settings RAGSettings) RAGSettings {
	if settings.Mode == "" {
		settings.Mode = RetrievalModeAuto
	}
	if settings.TopK == 0 {
		settings.TopK = DefaultRAGTopK
	}
	settings.KnowledgeBaseIDs = normalizeUniqueIDs(settings.KnowledgeBaseIDs)
	settings.CategoryIDs = normalizeUniqueIDs(settings.CategoryIDs)
	return settings
}

func ValidateRAGSettings(settings RAGSettings) error {
	settings = NormalizeRAGSettings(settings)
	switch settings.Mode {
	case RetrievalModeOff, RetrievalModeManual, RetrievalModeAuto, RetrievalModeAlways:
	default:
		return fmt.Errorf("unsupported retrieval mode: %s", settings.Mode)
	}
	if settings.TopK < 1 || settings.TopK > MaxRAGTopK {
		return fmt.Errorf("RAG top_k must be between 1 and %d", MaxRAGTopK)
	}
	return nil
}

func CloneRAGSettings(settings RAGSettings) RAGSettings {
	settings = NormalizeRAGSettings(settings)
	settings.KnowledgeBaseIDs = append([]string(nil), settings.KnowledgeBaseIDs...)
	settings.CategoryIDs = append([]string(nil), settings.CategoryIDs...)
	return settings
}

func normalizeUniqueIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
