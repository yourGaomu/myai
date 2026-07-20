package service

import (
	"context"
	"testing"
	"time"

	catalogcommand "myai/core/application/knowledge/catalog/command"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

func TestCatalogServiceRejectsDuplicateSiblingCategoryNames(t *testing.T) {
	fixtures := newCatalogFixtures()

	if _, err := fixtures.service.CreateCategory(context.Background(), catalogcommand.CreateCategory{Name: "Docs"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixtures.service.CreateCategory(context.Background(), catalogcommand.CreateCategory{Name: " docs "}); err == nil {
		t.Fatal("expected case-insensitive duplicate sibling name error")
	}
}

func TestCatalogServiceRejectsCategoryMoveBelowDescendant(t *testing.T) {
	fixtures := newCatalogFixtures()
	fixtures.categories.items["parent"] = domainknowledge.KnowledgeCategory{ID: "parent", Name: "Parent"}
	fixtures.categories.items["child"] = domainknowledge.KnowledgeCategory{
		ID: "child", Name: "Child", ParentID: "parent", AncestorIDs: []string{"parent"},
	}

	_, err := fixtures.service.MoveCategory(context.Background(), catalogcommand.MoveCategory{
		CategoryID: "parent",
		ParentID:   "child",
	})
	if err == nil {
		t.Fatal("expected category cycle error")
	}
}

func TestCatalogServiceRejectsCategoryBeyondMaximumDepth(t *testing.T) {
	fixtures := newCatalogFixtures()
	for level := 1; level <= domainknowledge.MaxCategoryDepth; level++ {
		id := categoryID(level)
		parentID := ""
		ancestors := make([]string, 0, level-1)
		for ancestor := 1; ancestor < level; ancestor++ {
			ancestors = append(ancestors, categoryID(ancestor))
		}
		if level > 1 {
			parentID = categoryID(level - 1)
		}
		fixtures.categories.items[id] = domainknowledge.KnowledgeCategory{
			ID: id, Name: id, ParentID: parentID, AncestorIDs: ancestors,
		}
	}

	_, err := fixtures.service.CreateCategory(context.Background(), catalogcommand.CreateCategory{
		Name:     "Too deep",
		ParentID: categoryID(domainknowledge.MaxCategoryDepth),
	})
	if err == nil {
		t.Fatal("expected maximum category depth error")
	}
}

func TestCatalogServiceRequiresExplicitRecursiveCategoryDeletion(t *testing.T) {
	fixtures := newCatalogFixtures()
	fixtures.categories.items["parent"] = domainknowledge.KnowledgeCategory{ID: "parent", Name: "Parent"}
	fixtures.categories.items["child"] = domainknowledge.KnowledgeCategory{
		ID: "child", Name: "Child", ParentID: "parent", AncestorIDs: []string{"parent"},
	}
	fixtures.bases.items["kb-1"] = domainknowledge.KnowledgeBase{
		ID: "kb-1", Name: "Project", CategoryID: "child", SyncSequence: 7,
	}

	err := fixtures.service.DeleteCategory(context.Background(), catalogcommand.DeleteCategory{CategoryID: "parent"})
	if err == nil {
		t.Fatal("expected non-empty category deletion error")
	}
	if fixtures.categories.items["parent"].Deletion.Deleted || fixtures.bases.items["kb-1"].Deletion.Deleted {
		t.Fatal("failed deletion must not modify resources")
	}

	err = fixtures.service.DeleteCategory(context.Background(), catalogcommand.DeleteCategory{
		CategoryID: "parent",
		Recursive:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fixtures.categories.items["parent"].Deletion.Deleted || !fixtures.categories.items["child"].Deletion.Deleted {
		t.Fatal("recursive deletion must logically delete the whole category subtree")
	}
	if !fixtures.bases.items["kb-1"].Deletion.Deleted {
		t.Fatal("recursive deletion must logically delete knowledge bases in the subtree")
	}
}

type catalogFixtures struct {
	service    CatalogService
	categories *catalogCategoryRepository
	bases      *catalogKnowledgeBaseRepository
}

func newCatalogFixtures() catalogFixtures {
	categories := &catalogCategoryRepository{items: map[string]domainknowledge.KnowledgeCategory{}}
	bases := &catalogKnowledgeBaseRepository{items: map[string]domainknowledge.KnowledgeBase{}}
	now := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC)
	return catalogFixtures{
		service: CatalogService{
			Categories:     categories,
			KnowledgeBases: bases,
			IDs:            &catalogIDGenerator{},
			Now:            func() time.Time { return now },
		},
		categories: categories,
		bases:      bases,
	}
}

func categoryID(level int) string {
	return "category-" + string(rune('0'+level))
}

type catalogIDGenerator struct {
	next int
}

func (generator *catalogIDGenerator) NewID() string {
	generator.next++
	return categoryID(generator.next)
}

type catalogCategoryRepository struct {
	items map[string]domainknowledge.KnowledgeCategory
}

func (repository *catalogCategoryRepository) Get(_ context.Context, categoryID string) (domainknowledge.KnowledgeCategory, error) {
	category, ok := repository.items[categoryID]
	if !ok || category.Deletion.Deleted {
		return domainknowledge.KnowledgeCategory{}, knowledgeport.ErrNotFound
	}
	return category, nil
}

func (repository *catalogCategoryRepository) List(_ context.Context, includeDeleted bool) ([]domainknowledge.KnowledgeCategory, error) {
	result := make([]domainknowledge.KnowledgeCategory, 0, len(repository.items))
	for _, category := range repository.items {
		if includeDeleted || !category.Deletion.Deleted {
			category.AncestorIDs = append([]string(nil), category.AncestorIDs...)
			result = append(result, category)
		}
	}
	return result, nil
}

func (repository *catalogCategoryRepository) Save(_ context.Context, category domainknowledge.KnowledgeCategory) error {
	category.AncestorIDs = append([]string(nil), category.AncestorIDs...)
	repository.items[category.ID] = category
	return nil
}

func (repository *catalogCategoryRepository) MarkDeleted(_ context.Context, categoryID string, deletedAt time.Time, reason string) error {
	category, ok := repository.items[categoryID]
	if !ok {
		return knowledgeport.ErrNotFound
	}
	category.Deletion = domainknowledge.Deletion{Deleted: true, DeletedAt: &deletedAt, DeleteReason: reason}
	repository.items[categoryID] = category
	return nil
}

type catalogKnowledgeBaseRepository struct {
	items map[string]domainknowledge.KnowledgeBase
}

func (repository *catalogKnowledgeBaseRepository) Get(_ context.Context, knowledgeBaseID string) (domainknowledge.KnowledgeBase, error) {
	base, ok := repository.items[knowledgeBaseID]
	if !ok || base.Deletion.Deleted {
		return domainknowledge.KnowledgeBase{}, knowledgeport.ErrNotFound
	}
	return base, nil
}

func (repository *catalogKnowledgeBaseRepository) List(_ context.Context, includeDeleted bool) ([]domainknowledge.KnowledgeBase, error) {
	result := make([]domainknowledge.KnowledgeBase, 0, len(repository.items))
	for _, base := range repository.items {
		if includeDeleted || !base.Deletion.Deleted {
			result = append(result, base)
		}
	}
	return result, nil
}

func (repository *catalogKnowledgeBaseRepository) Save(_ context.Context, base domainknowledge.KnowledgeBase) error {
	repository.items[base.ID] = base
	return nil
}

func (repository *catalogKnowledgeBaseRepository) MarkDeleted(_ context.Context, knowledgeBaseID string, deletedAt time.Time, reason string, _ int64) error {
	base, ok := repository.items[knowledgeBaseID]
	if !ok {
		return knowledgeport.ErrNotFound
	}
	base.Deletion = domainknowledge.Deletion{Deleted: true, DeletedAt: &deletedAt, DeleteReason: reason}
	repository.items[knowledgeBaseID] = base
	return nil
}
