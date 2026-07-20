package service

import (
	"fmt"

	retrievalport "myai/core/application/knowledge/retrieval/port"
	knowledgeport "myai/core/port/knowledge"
)

const (
	defaultCandidateMultiplier = 3
	defaultMaxCandidates       = 100
	defaultMinLocalResults     = 3
	defaultMinLocalScore       = 0.55
	defaultRRFK                = 60
)

type Configuration struct {
	KnowledgeBases      knowledgeport.KnowledgeBaseRepository
	Documents           knowledgeport.DocumentRepository
	Profiles            knowledgeport.ProfileRepository
	Chunks              knowledgeport.ChunkRepository
	Embeddings          knowledgeport.EmbeddingModelResolver
	LocalVectors        knowledgeport.VectorStore
	LocalKeywords       knowledgeport.KeywordStore
	RemoteVectors       knowledgeport.VectorStore
	QualityGate         retrievalport.LocalQualityGate
	Fusion              retrievalport.RankFusion
	CandidateMultiplier int
	MaxCandidates       int
	MinLocalResults     int
	MinLocalScore       float64
	RRFK                int
	CacheRemoteResults  bool
}

func (configuration Configuration) normalize() Configuration {
	if configuration.CandidateMultiplier == 0 {
		configuration.CandidateMultiplier = defaultCandidateMultiplier
	}
	if configuration.MaxCandidates == 0 {
		configuration.MaxCandidates = defaultMaxCandidates
	}
	if configuration.MinLocalResults == 0 {
		configuration.MinLocalResults = defaultMinLocalResults
	}
	if configuration.MinLocalScore == 0 {
		configuration.MinLocalScore = defaultMinLocalScore
	}
	if configuration.RRFK == 0 {
		configuration.RRFK = defaultRRFK
	}
	return configuration
}

func (configuration Configuration) validate() error {
	dependencies := []struct {
		name  string
		value any
	}{
		{"knowledge base repository", configuration.KnowledgeBases},
		{"document repository", configuration.Documents},
		{"profile repository", configuration.Profiles},
		{"chunk repository", configuration.Chunks},
		{"embedding model resolver", configuration.Embeddings},
	}
	for _, dependency := range dependencies {
		if dependency.value == nil {
			return fmt.Errorf("retrieval %s is nil", dependency.name)
		}
	}
	if configuration.LocalVectors == nil && configuration.LocalKeywords == nil && configuration.RemoteVectors == nil {
		return fmt.Errorf("retrieval requires at least one search store")
	}
	if configuration.CandidateMultiplier < 1 {
		return fmt.Errorf("retrieval candidate multiplier must be positive")
	}
	if configuration.MaxCandidates < 1 {
		return fmt.Errorf("retrieval max candidates must be positive")
	}
	return nil
}
