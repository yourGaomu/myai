package service

import (
	"context"
	"fmt"
	"math"
	"strings"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type Resolver struct {
	Registry knowledgeport.EmbeddingModelRegistry
}

var _ knowledgeport.EmbeddingModelResolver = Resolver{}

func (resolver Resolver) Resolve(profile domainknowledge.EmbeddingProfile) (knowledgeport.EmbeddingProvider, error) {
	if resolver.Registry == nil {
		return nil, fmt.Errorf("embedding model registry is nil")
	}
	if err := profile.Validate(); err != nil {
		return nil, fmt.Errorf("validate embedding profile: %w", err)
	}
	if profile.Deletion.Deleted {
		return nil, fmt.Errorf("embedding profile %q is deleted", profile.ID)
	}
	provider, exists := resolver.Registry.Get(profile.ModelID)
	if !exists || provider == nil {
		return nil, fmt.Errorf("embedding model %q is not registered", profile.ModelID)
	}
	info, exists := resolver.Registry.GetInfo(profile.ModelID)
	if !exists {
		return nil, fmt.Errorf("embedding model metadata %q is not registered", profile.ModelID)
	}
	if err := validateProfileCompatibility(profile, info); err != nil {
		return nil, err
	}
	return &profileProvider{delegate: provider, profile: profile}, nil
}

func validateProfileCompatibility(profile domainknowledge.EmbeddingProfile, info domainknowledge.EmbeddingModelInfo) error {
	if err := info.Validate(); err != nil {
		return fmt.Errorf("validate embedding model metadata %q: %w", info.ID, err)
	}
	if !info.Enabled {
		return fmt.Errorf("embedding model %q is disabled", info.ID)
	}
	if profile.ModelID != info.ID {
		return fmt.Errorf("embedding profile model id %q does not match registered model %q", profile.ModelID, info.ID)
	}
	if !strings.EqualFold(strings.TrimSpace(profile.Provider), strings.TrimSpace(info.Provider)) {
		return fmt.Errorf("embedding profile provider %q does not match registered provider %q", profile.Provider, info.Provider)
	}
	if strings.TrimSpace(profile.Model) != strings.TrimSpace(info.Model) {
		return fmt.Errorf("embedding profile model %q does not match registered model %q", profile.Model, info.Model)
	}
	if strings.TrimSpace(profile.ModelVersion) != strings.TrimSpace(info.ModelVersion) {
		return fmt.Errorf("embedding profile model version %q does not match registered version %q", profile.ModelVersion, info.ModelVersion)
	}
	if profile.Dimensions != info.Dimensions {
		return fmt.Errorf("embedding profile dimensions %d do not match registered dimensions %d", profile.Dimensions, info.Dimensions)
	}
	return nil
}

type profileProvider struct {
	delegate knowledgeport.EmbeddingProvider
	profile  domainknowledge.EmbeddingProfile
}

func (provider *profileProvider) EmbedDocuments(ctx context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	if err := provider.validateRequest(request); err != nil {
		return domainknowledge.EmbedResult{}, err
	}
	result, err := provider.delegate.EmbedDocuments(ctx, request)
	if err != nil {
		return domainknowledge.EmbedResult{}, err
	}
	return provider.prepareResult(result)
}

func (provider *profileProvider) EmbedQuery(ctx context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	if err := provider.validateRequest(request); err != nil {
		return domainknowledge.EmbedResult{}, err
	}
	result, err := provider.delegate.EmbedQuery(ctx, request)
	if err != nil {
		return domainknowledge.EmbedResult{}, err
	}
	return provider.prepareResult(result)
}

func (provider *profileProvider) validateRequest(request domainknowledge.EmbedRequest) error {
	if request.EmbeddingProfileID != provider.profile.ID {
		return fmt.Errorf("embedding request profile %q does not match resolved profile %q", request.EmbeddingProfileID, provider.profile.ID)
	}
	return nil
}

func (provider *profileProvider) prepareResult(result domainknowledge.EmbedResult) (domainknowledge.EmbedResult, error) {
	if err := result.Validate(); err != nil {
		return domainknowledge.EmbedResult{}, fmt.Errorf("validate embedding result: %w", err)
	}
	if result.EmbeddingProfileID != provider.profile.ID {
		return domainknowledge.EmbedResult{}, fmt.Errorf("embedding result profile %q does not match resolved profile %q", result.EmbeddingProfileID, provider.profile.ID)
	}
	if result.Dimensions != provider.profile.Dimensions {
		return domainknowledge.EmbedResult{}, fmt.Errorf("embedding result dimensions %d do not match profile dimensions %d", result.Dimensions, provider.profile.Dimensions)
	}
	prepared := domainknowledge.EmbedResult{
		EmbeddingProfileID: result.EmbeddingProfileID,
		Dimensions:         result.Dimensions,
		Outputs:            make([]domainknowledge.EmbedOutput, 0, len(result.Outputs)),
	}
	for _, output := range result.Outputs {
		vector := append([]float32(nil), output.Vector...)
		if provider.profile.Normalize {
			if err := normalizeL2(vector); err != nil {
				return domainknowledge.EmbedResult{}, fmt.Errorf("normalize embedding output %q: %w", output.ID, err)
			}
		}
		prepared.Outputs = append(prepared.Outputs, domainknowledge.EmbedOutput{ID: output.ID, Vector: vector})
	}
	if err := prepared.Validate(); err != nil {
		return domainknowledge.EmbedResult{}, fmt.Errorf("validate prepared embedding result: %w", err)
	}
	return prepared, nil
}

func normalizeL2(vector []float32) error {
	var squaredSum float64
	for _, value := range vector {
		converted := float64(value)
		if math.IsNaN(converted) || math.IsInf(converted, 0) {
			return fmt.Errorf("embedding vector must contain finite values")
		}
		squaredSum += converted * converted
	}
	if squaredSum == 0 {
		return fmt.Errorf("zero embedding vector cannot be normalized")
	}
	norm := math.Sqrt(squaredSum)
	for index := range vector {
		vector[index] = float32(float64(vector[index]) / norm)
	}
	return nil
}
