package service

import (
	"context"
	"fmt"

	domainknowledge "myai/core/domain/knowledge"
)

type loadedProfiles struct {
	index     domainknowledge.IndexProfile
	parsing   domainknowledge.ParsingProfile
	chunking  domainknowledge.ChunkingProfile
	embedding domainknowledge.EmbeddingProfile
}

func (service *IndexingService) loadProfiles(ctx context.Context, indexProfileID string) (loadedProfiles, error) {
	indexProfile, err := service.configuration.Profiles.GetIndexProfile(ctx, indexProfileID)
	if err != nil {
		return loadedProfiles{}, fmt.Errorf("load index profile %q: %w", indexProfileID, err)
	}
	if err := validateActiveIndexProfile(indexProfile); err != nil {
		return loadedProfiles{}, err
	}
	parsingProfile, err := service.configuration.Profiles.GetParsingProfile(ctx, indexProfile.ParsingProfileID)
	if err != nil {
		return loadedProfiles{}, fmt.Errorf("load parsing profile %q: %w", indexProfile.ParsingProfileID, err)
	}
	if err := parsingProfile.Validate(); err != nil {
		return loadedProfiles{}, fmt.Errorf("validate parsing profile %q: %w", parsingProfile.ID, err)
	}
	if parsingProfile.ID != indexProfile.ParsingProfileID || parsingProfile.Deletion.Deleted {
		return loadedProfiles{}, fmt.Errorf("parsing profile %q is unavailable", indexProfile.ParsingProfileID)
	}
	chunkingProfile, err := service.configuration.Profiles.GetChunkingProfile(ctx, indexProfile.ChunkingProfileID)
	if err != nil {
		return loadedProfiles{}, fmt.Errorf("load chunking profile %q: %w", indexProfile.ChunkingProfileID, err)
	}
	if err := chunkingProfile.Validate(); err != nil {
		return loadedProfiles{}, fmt.Errorf("validate chunking profile %q: %w", chunkingProfile.ID, err)
	}
	if chunkingProfile.ID != indexProfile.ChunkingProfileID || chunkingProfile.Deletion.Deleted {
		return loadedProfiles{}, fmt.Errorf("chunking profile %q is unavailable", indexProfile.ChunkingProfileID)
	}
	embeddingProfile, err := service.configuration.Profiles.GetEmbeddingProfile(ctx, indexProfile.EmbeddingProfileID)
	if err != nil {
		return loadedProfiles{}, fmt.Errorf("load embedding profile %q: %w", indexProfile.EmbeddingProfileID, err)
	}
	if err := embeddingProfile.Validate(); err != nil {
		return loadedProfiles{}, fmt.Errorf("validate embedding profile %q: %w", embeddingProfile.ID, err)
	}
	if embeddingProfile.ID != indexProfile.EmbeddingProfileID || embeddingProfile.Deletion.Deleted {
		return loadedProfiles{}, fmt.Errorf("embedding profile %q is unavailable", indexProfile.EmbeddingProfileID)
	}
	return loadedProfiles{
		index:     indexProfile,
		parsing:   parsingProfile,
		chunking:  chunkingProfile,
		embedding: embeddingProfile,
	}, nil
}

func validateIndexableDocument(document domainknowledge.Document) error {
	if err := document.Validate(); err != nil {
		return fmt.Errorf("validate indexing document %q: %w", document.ID, err)
	}
	if document.Deletion.Deleted || document.Status == domainknowledge.DocumentStatusDeleted {
		return fmt.Errorf("document %q is deleted", document.ID)
	}
	return nil
}

func validateActiveIndexProfile(profile domainknowledge.IndexProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validate index profile %q: %w", profile.ID, err)
	}
	if profile.Deletion.Deleted {
		return fmt.Errorf("index profile %q is deleted", profile.ID)
	}
	if profile.Status != domainknowledge.IndexProfileStatusActive {
		return fmt.Errorf("index profile %q is not active", profile.ID)
	}
	return nil
}

func validateJobDocument(job domainknowledge.IndexingJob, document domainknowledge.Document) error {
	if err := job.Validate(); err != nil {
		return fmt.Errorf("validate indexing job %q: %w", job.ID, err)
	}
	if err := validateIndexableDocument(document); err != nil {
		return err
	}
	if job.DocumentID != document.ID || job.KnowledgeBaseID != document.KnowledgeBaseID {
		return fmt.Errorf("indexing job %q does not match document %q", job.ID, document.ID)
	}
	return nil
}
