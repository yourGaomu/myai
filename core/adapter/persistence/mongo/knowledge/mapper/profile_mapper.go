package mapper

import (
	"myai/core/adapter/persistence/mongo/knowledge/po"
	domainknowledge "myai/core/domain/knowledge"
)

func ParsingProfileDocumentFromDomain(profile domainknowledge.ParsingProfile) po.ParsingProfileDocument {
	return po.ParsingProfileDocument{
		ID:            profile.ID,
		Name:          profile.Name,
		ParserID:      profile.ParserID,
		ParserVersion: profile.ParserVersion,
		OCR:           profile.OCR,
		Options:       cloneStringMap(profile.Options),
		Deleted:       profile.Deletion.Deleted,
		DeletedAt:     cloneTime(profile.Deletion.DeletedAt),
		DeleteReason:  profile.Deletion.DeleteReason,
		CreatedAt:     profile.CreatedAt,
		UpdatedAt:     profile.UpdatedAt,
	}
}

func ParsingProfileDomainFromDocument(document po.ParsingProfileDocument) domainknowledge.ParsingProfile {
	return domainknowledge.ParsingProfile{
		ID:            document.ID,
		Name:          document.Name,
		ParserID:      document.ParserID,
		ParserVersion: document.ParserVersion,
		OCR:           document.OCR,
		Options:       cloneStringMap(document.Options),
		Deletion: domainknowledge.Deletion{
			Deleted:      document.Deleted,
			DeletedAt:    cloneTime(document.DeletedAt),
			DeleteReason: document.DeleteReason,
		},
		CreatedAt: document.CreatedAt,
		UpdatedAt: document.UpdatedAt,
	}
}

func EmbeddingProfileDocumentFromDomain(profile domainknowledge.EmbeddingProfile) po.EmbeddingProfileDocument {
	return po.EmbeddingProfileDocument{
		ID:           profile.ID,
		Name:         profile.Name,
		ModelID:      profile.ModelID,
		Provider:     profile.Provider,
		Model:        profile.Model,
		ModelVersion: profile.ModelVersion,
		Dimensions:   profile.Dimensions,
		Normalize:    profile.Normalize,
		InputMode:    profile.InputMode,
		Deleted:      profile.Deletion.Deleted,
		DeletedAt:    cloneTime(profile.Deletion.DeletedAt),
		DeleteReason: profile.Deletion.DeleteReason,
		CreatedAt:    profile.CreatedAt,
		UpdatedAt:    profile.UpdatedAt,
	}
}

func EmbeddingProfileDomainFromDocument(document po.EmbeddingProfileDocument) domainknowledge.EmbeddingProfile {
	return domainknowledge.EmbeddingProfile{
		ID:           document.ID,
		Name:         document.Name,
		ModelID:      document.ModelID,
		Provider:     document.Provider,
		Model:        document.Model,
		ModelVersion: document.ModelVersion,
		Dimensions:   document.Dimensions,
		Normalize:    document.Normalize,
		InputMode:    document.InputMode,
		Deletion: domainknowledge.Deletion{
			Deleted:      document.Deleted,
			DeletedAt:    cloneTime(document.DeletedAt),
			DeleteReason: document.DeleteReason,
		},
		CreatedAt: document.CreatedAt,
		UpdatedAt: document.UpdatedAt,
	}
}

func ChunkingProfileDocumentFromDomain(profile domainknowledge.ChunkingProfile) po.ChunkingProfileDocument {
	return po.ChunkingProfileDocument{
		ID:              profile.ID,
		Name:            profile.Name,
		StrategyID:      profile.StrategyID,
		StrategyVersion: profile.StrategyVersion,
		MaxChunkSize:    profile.MaxChunkSize,
		Overlap:         profile.Overlap,
		Tokenizer:       profile.Tokenizer,
		Options:         cloneStringMap(profile.Options),
		Deleted:         profile.Deletion.Deleted,
		DeletedAt:       cloneTime(profile.Deletion.DeletedAt),
		DeleteReason:    profile.Deletion.DeleteReason,
		CreatedAt:       profile.CreatedAt,
		UpdatedAt:       profile.UpdatedAt,
	}
}

func ChunkingProfileDomainFromDocument(document po.ChunkingProfileDocument) domainknowledge.ChunkingProfile {
	return domainknowledge.ChunkingProfile{
		ID:              document.ID,
		Name:            document.Name,
		StrategyID:      document.StrategyID,
		StrategyVersion: document.StrategyVersion,
		MaxChunkSize:    document.MaxChunkSize,
		Overlap:         document.Overlap,
		Tokenizer:       document.Tokenizer,
		Options:         cloneStringMap(document.Options),
		Deletion: domainknowledge.Deletion{
			Deleted:      document.Deleted,
			DeletedAt:    cloneTime(document.DeletedAt),
			DeleteReason: document.DeleteReason,
		},
		CreatedAt: document.CreatedAt,
		UpdatedAt: document.UpdatedAt,
	}
}

func IndexProfileDocumentFromDomain(profile domainknowledge.IndexProfile) po.IndexProfileDocument {
	return po.IndexProfileDocument{
		ID:                 profile.ID,
		Name:               profile.Name,
		ParsingProfileID:   profile.ParsingProfileID,
		ChunkingProfileID:  profile.ChunkingProfileID,
		EmbeddingProfileID: profile.EmbeddingProfileID,
		DistanceMetricID:   profile.DistanceMetricID,
		VectorIndexOptions: cloneStringMap(profile.VectorIndexOptions),
		Status:             string(profile.Status),
		FailureReason:      profile.FailureReason,
		Deleted:            profile.Deletion.Deleted,
		DeletedAt:          cloneTime(profile.Deletion.DeletedAt),
		DeleteReason:       profile.Deletion.DeleteReason,
		CreatedAt:          profile.CreatedAt,
		UpdatedAt:          profile.UpdatedAt,
	}
}

func IndexProfileDomainFromDocument(document po.IndexProfileDocument) domainknowledge.IndexProfile {
	return domainknowledge.IndexProfile{
		ID:                 document.ID,
		Name:               document.Name,
		ParsingProfileID:   document.ParsingProfileID,
		ChunkingProfileID:  document.ChunkingProfileID,
		EmbeddingProfileID: document.EmbeddingProfileID,
		DistanceMetricID:   document.DistanceMetricID,
		VectorIndexOptions: cloneStringMap(document.VectorIndexOptions),
		Status:             domainknowledge.IndexProfileStatus(document.Status),
		FailureReason:      document.FailureReason,
		Deletion: domainknowledge.Deletion{
			Deleted:      document.Deleted,
			DeletedAt:    cloneTime(document.DeletedAt),
			DeleteReason: document.DeleteReason,
		},
		CreatedAt: document.CreatedAt,
		UpdatedAt: document.UpdatedAt,
	}
}
