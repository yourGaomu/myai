package service

import (
	"fmt"
	"time"

	knowledgeport "myai/core/port/knowledge"
	documentprocessorport "myai/core/port/knowledge/documentprocessor"
)

type Configuration struct {
	Documents  knowledgeport.DocumentRepository
	Profiles   knowledgeport.ProfileRepository
	Jobs       knowledgeport.IndexingJobRepository
	States     knowledgeport.IndexingStateRepository
	Chunks     knowledgeport.ChunkRepository
	Objects    knowledgeport.DocumentObjectStore
	Processor  documentprocessorport.DocumentProcessor
	Embeddings knowledgeport.EmbeddingModelResolver
	Vectors    knowledgeport.VectorStore
	Keywords   knowledgeport.KeywordStore
	JobIDs     knowledgeport.IDGenerator
	StableIDs  knowledgeport.StableIDDeriver
	PageSize   int
	Now        func() time.Time
}

func (configuration Configuration) normalize() Configuration {
	if configuration.PageSize == 0 {
		configuration.PageSize = 64
	}
	if configuration.Now == nil {
		configuration.Now = time.Now
	}
	return configuration
}

func (configuration Configuration) validate() error {
	dependencies := []struct {
		name  string
		value any
	}{
		{"document repository", configuration.Documents},
		{"profile repository", configuration.Profiles},
		{"indexing job repository", configuration.Jobs},
		{"indexing state repository", configuration.States},
		{"chunk repository", configuration.Chunks},
		{"document object store", configuration.Objects},
		{"document processor", configuration.Processor},
		{"embedding model resolver", configuration.Embeddings},
		{"vector store", configuration.Vectors},
		{"keyword store", configuration.Keywords},
		{"job id generator", configuration.JobIDs},
		{"stable id deriver", configuration.StableIDs},
	}
	for _, dependency := range dependencies {
		if dependency.value == nil {
			return fmt.Errorf("indexing %s is nil", dependency.name)
		}
	}
	if configuration.PageSize < 1 {
		return fmt.Errorf("indexing page size must be positive")
	}
	return nil
}
