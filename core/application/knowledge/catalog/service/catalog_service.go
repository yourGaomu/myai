package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	catalogapi "myai/core/application/knowledge/catalog/api"
	catalogcommand "myai/core/application/knowledge/catalog/command"
	catalogresult "myai/core/application/knowledge/catalog/result"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type CatalogService struct {
	Categories     knowledgeport.KnowledgeCategoryRepository
	KnowledgeBases knowledgeport.KnowledgeBaseRepository
	Profiles       knowledgeport.ProfileRepository
	IDs            knowledgeport.IDGenerator
	Now            func() time.Time
}

var _ catalogapi.Service = CatalogService{}

func (service CatalogService) List(ctx context.Context, command catalogcommand.List) (catalogresult.Catalog, error) {
	if err := service.validateCoreDependencies(); err != nil {
		return catalogresult.Catalog{}, err
	}
	categories, err := service.Categories.List(ctx, command.IncludeDeleted)
	if err != nil {
		return catalogresult.Catalog{}, err
	}
	bases, err := service.KnowledgeBases.List(ctx, command.IncludeDeleted)
	if err != nil {
		return catalogresult.Catalog{}, err
	}
	sort.SliceStable(categories, func(left, right int) bool {
		if categories[left].ParentID != categories[right].ParentID {
			return categories[left].ParentID < categories[right].ParentID
		}
		if categories[left].SortOrder != categories[right].SortOrder {
			return categories[left].SortOrder < categories[right].SortOrder
		}
		return categories[left].Name < categories[right].Name
	})
	return catalogresult.Catalog{Categories: categories, KnowledgeBases: bases}, nil
}

func (service CatalogService) CreateCategory(ctx context.Context, command catalogcommand.CreateCategory) (catalogresult.Category, error) {
	if err := service.validateCoreDependencies(); err != nil {
		return catalogresult.Category{}, err
	}
	name := strings.TrimSpace(command.Name)
	parentID := strings.TrimSpace(command.ParentID)
	categories, err := service.Categories.List(ctx, false)
	if err != nil {
		return catalogresult.Category{}, err
	}
	if categoryNameExists(categories, "", parentID, name) {
		return catalogresult.Category{}, fmt.Errorf("knowledge category %q already exists under the selected parent", name)
	}
	ancestors, err := categoryAncestors(categories, parentID)
	if err != nil {
		return catalogresult.Category{}, err
	}
	now := service.now()
	category := domainknowledge.KnowledgeCategory{
		ID: service.IDs.NewID(), Name: name, ParentID: parentID,
		AncestorIDs: ancestors, SortOrder: command.SortOrder, CreatedAt: now, UpdatedAt: now,
	}
	if err := category.Validate(); err != nil {
		return catalogresult.Category{}, err
	}
	if err := service.Categories.Save(ctx, category); err != nil {
		return catalogresult.Category{}, err
	}
	return catalogresult.Category{Category: category}, nil
}

func (service CatalogService) MoveCategory(ctx context.Context, command catalogcommand.MoveCategory) (catalogresult.Category, error) {
	if err := service.validateCoreDependencies(); err != nil {
		return catalogresult.Category{}, err
	}
	categoryID := strings.TrimSpace(command.CategoryID)
	parentID := strings.TrimSpace(command.ParentID)
	if categoryID == "" {
		return catalogresult.Category{}, errors.New("knowledge category id is required")
	}
	if categoryID == parentID {
		return catalogresult.Category{}, errors.New("knowledge category cannot be its own parent")
	}
	categories, err := service.Categories.List(ctx, false)
	if err != nil {
		return catalogresult.Category{}, err
	}
	current, ok := categoryByID(categories, categoryID)
	if !ok {
		return catalogresult.Category{}, knowledgeport.ErrNotFound
	}
	if categoryNameExists(categories, current.ID, parentID, current.Name) {
		return catalogresult.Category{}, fmt.Errorf("knowledge category %q already exists under the selected parent", current.Name)
	}
	parentAncestors, err := categoryAncestors(categories, parentID)
	if err != nil {
		return catalogresult.Category{}, err
	}
	for _, ancestorID := range parentAncestors {
		if ancestorID == categoryID {
			return catalogresult.Category{}, errors.New("knowledge category cannot be moved below its descendant")
		}
	}

	oldPrefix := append(append([]string(nil), current.AncestorIDs...), current.ID)
	current.ParentID = parentID
	current.AncestorIDs = parentAncestors
	current.SortOrder = command.SortOrder
	current.UpdatedAt = service.now()
	updates := []domainknowledge.KnowledgeCategory{current}
	for _, candidate := range categories {
		if candidate.ID == current.ID || !hasAncestor(candidate, current.ID) {
			continue
		}
		if len(candidate.AncestorIDs) < len(oldPrefix) {
			return catalogresult.Category{}, fmt.Errorf("invalid ancestor path for category %q", candidate.ID)
		}
		suffix := append([]string(nil), candidate.AncestorIDs[len(oldPrefix):]...)
		candidate.AncestorIDs = append(append(append([]string(nil), parentAncestors...), current.ID), suffix...)
		candidate.UpdatedAt = current.UpdatedAt
		updates = append(updates, candidate)
	}
	for _, update := range updates {
		if err := update.Validate(); err != nil {
			return catalogresult.Category{}, err
		}
	}
	for _, update := range updates {
		if err := service.Categories.Save(ctx, update); err != nil {
			return catalogresult.Category{}, err
		}
	}
	return catalogresult.Category{Category: current}, nil
}

func (service CatalogService) DeleteCategory(ctx context.Context, command catalogcommand.DeleteCategory) error {
	if err := service.validateCoreDependencies(); err != nil {
		return err
	}
	categoryID := strings.TrimSpace(command.CategoryID)
	if categoryID == "" {
		return errors.New("knowledge category id is required")
	}
	categories, err := service.Categories.List(ctx, false)
	if err != nil {
		return err
	}
	if _, ok := categoryByID(categories, categoryID); !ok {
		return knowledgeport.ErrNotFound
	}
	bases, err := service.KnowledgeBases.List(ctx, false)
	if err != nil {
		return err
	}
	categoryIDs := map[string]struct{}{categoryID: {}}
	for _, category := range categories {
		if hasAncestor(category, categoryID) {
			categoryIDs[category.ID] = struct{}{}
		}
	}
	hasChildren := len(categoryIDs) > 1
	hasBases := false
	for _, base := range bases {
		if _, exists := categoryIDs[base.CategoryID]; exists {
			hasBases = true
			break
		}
	}
	if !command.Recursive && (hasChildren || hasBases) {
		return errors.New("knowledge category is not empty; recursive deletion must be explicitly enabled")
	}
	now := service.now()
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		reason = "category deleted"
	}
	for _, base := range bases {
		if _, exists := categoryIDs[base.CategoryID]; exists {
			if err := service.KnowledgeBases.MarkDeleted(ctx, base.ID, now, reason, base.SyncSequence); err != nil {
				return err
			}
		}
	}
	for _, category := range categories {
		if _, exists := categoryIDs[category.ID]; exists {
			if err := service.Categories.MarkDeleted(ctx, category.ID, now, reason); err != nil {
				return err
			}
		}
	}
	return nil
}

func (service CatalogService) CreateKnowledgeBase(ctx context.Context, command catalogcommand.CreateKnowledgeBase) (catalogresult.KnowledgeBase, error) {
	if err := service.validateCoreDependencies(); err != nil {
		return catalogresult.KnowledgeBase{}, err
	}
	base := domainknowledge.KnowledgeBase{
		ID: service.IDs.NewID(), CategoryID: strings.TrimSpace(command.CategoryID),
		Name: strings.TrimSpace(command.Name), Description: strings.TrimSpace(command.Description),
		RAGEnabled: command.RAGEnabled, ActiveIndexProfileID: strings.TrimSpace(command.ActiveIndexProfileID),
		CreatedAt: service.now(), UpdatedAt: service.now(),
	}
	if err := service.validateKnowledgeBase(ctx, base, ""); err != nil {
		return catalogresult.KnowledgeBase{}, err
	}
	if err := service.KnowledgeBases.Save(ctx, base); err != nil {
		return catalogresult.KnowledgeBase{}, err
	}
	return catalogresult.KnowledgeBase{KnowledgeBase: base}, nil
}

func (service CatalogService) UpdateKnowledgeBase(ctx context.Context, command catalogcommand.UpdateKnowledgeBase) (catalogresult.KnowledgeBase, error) {
	if err := service.validateCoreDependencies(); err != nil {
		return catalogresult.KnowledgeBase{}, err
	}
	base, err := service.KnowledgeBases.Get(ctx, strings.TrimSpace(command.KnowledgeBaseID))
	if err != nil {
		return catalogresult.KnowledgeBase{}, err
	}
	base.CategoryID = strings.TrimSpace(command.CategoryID)
	base.Name = strings.TrimSpace(command.Name)
	base.Description = strings.TrimSpace(command.Description)
	base.RAGEnabled = command.RAGEnabled
	base.ActiveIndexProfileID = strings.TrimSpace(command.ActiveIndexProfileID)
	base.UpdatedAt = service.now()
	if err := service.validateKnowledgeBase(ctx, base, base.ID); err != nil {
		return catalogresult.KnowledgeBase{}, err
	}
	if err := service.KnowledgeBases.Save(ctx, base); err != nil {
		return catalogresult.KnowledgeBase{}, err
	}
	return catalogresult.KnowledgeBase{KnowledgeBase: base}, nil
}

func (service CatalogService) DeleteKnowledgeBase(ctx context.Context, command catalogcommand.DeleteKnowledgeBase) error {
	if err := service.validateCoreDependencies(); err != nil {
		return err
	}
	base, err := service.KnowledgeBases.Get(ctx, strings.TrimSpace(command.KnowledgeBaseID))
	if err != nil {
		return err
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		reason = "knowledge base deleted"
	}
	return service.KnowledgeBases.MarkDeleted(ctx, base.ID, service.now(), reason, base.SyncSequence)
}

func (service CatalogService) validateKnowledgeBase(ctx context.Context, base domainknowledge.KnowledgeBase, excludedID string) error {
	if err := base.Validate(); err != nil {
		return err
	}
	if base.CategoryID != "" {
		if _, err := service.Categories.Get(ctx, base.CategoryID); err != nil {
			return fmt.Errorf("load knowledge category %q: %w", base.CategoryID, err)
		}
	}
	bases, err := service.KnowledgeBases.List(ctx, false)
	if err != nil {
		return err
	}
	for _, candidate := range bases {
		if candidate.ID != excludedID && candidate.CategoryID == base.CategoryID && strings.EqualFold(strings.TrimSpace(candidate.Name), base.Name) {
			return fmt.Errorf("knowledge base %q already exists in the selected category", base.Name)
		}
	}
	if base.RAGEnabled {
		if service.Profiles == nil {
			return errors.New("knowledge profile repository is nil")
		}
		profile, err := service.Profiles.GetIndexProfile(ctx, base.ActiveIndexProfileID)
		if err != nil {
			return fmt.Errorf("load index profile %q: %w", base.ActiveIndexProfileID, err)
		}
		if err := profile.Validate(); err != nil {
			return err
		}
		if profile.Status != domainknowledge.IndexProfileStatusActive || profile.Deletion.Deleted {
			return fmt.Errorf("index profile %q is not active", profile.ID)
		}
	}
	return nil
}

func (service CatalogService) validateCoreDependencies() error {
	if service.Categories == nil {
		return errors.New("knowledge category repository is nil")
	}
	if service.KnowledgeBases == nil {
		return errors.New("knowledge base repository is nil")
	}
	if service.IDs == nil {
		return errors.New("knowledge id generator is nil")
	}
	return nil
}

func (service CatalogService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func categoryByID(categories []domainknowledge.KnowledgeCategory, categoryID string) (domainknowledge.KnowledgeCategory, bool) {
	for _, category := range categories {
		if category.ID == categoryID {
			return category, true
		}
	}
	return domainknowledge.KnowledgeCategory{}, false
}

func categoryAncestors(categories []domainknowledge.KnowledgeCategory, parentID string) ([]string, error) {
	if parentID == "" {
		return nil, nil
	}
	parent, ok := categoryByID(categories, parentID)
	if !ok {
		return nil, fmt.Errorf("knowledge category parent %q: %w", parentID, knowledgeport.ErrNotFound)
	}
	return append(append([]string(nil), parent.AncestorIDs...), parent.ID), nil
}

func categoryNameExists(categories []domainknowledge.KnowledgeCategory, excludedID string, parentID string, name string) bool {
	for _, category := range categories {
		if category.ID != excludedID && category.ParentID == parentID && strings.EqualFold(strings.TrimSpace(category.Name), name) {
			return true
		}
	}
	return false
}

func hasAncestor(category domainknowledge.KnowledgeCategory, ancestorID string) bool {
	for _, candidate := range category.AncestorIDs {
		if candidate == ancestorID {
			return true
		}
	}
	return false
}
