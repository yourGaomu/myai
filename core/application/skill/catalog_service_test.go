package skillapp

import (
	"context"
	"errors"
	"testing"

	domainskill "myai/core/skill"
)

func TestCatalogServiceRefreshesAndReturnsSkillCopies(t *testing.T) {
	catalog := &fakeSkillCatalog{
		root:   " skills ",
		skills: []domainskill.Skill{{Name: "review"}},
	}
	service := CatalogService{Catalog: catalog}

	result, err := service.List(context.Background(), ListSkillsQuery{Refresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.reloaded || len(result.Skills) != 1 || result.Skills[0].Name != "review" {
		t.Fatalf("unexpected result or refresh state: result=%#v catalog=%#v", result, catalog)
	}
	result.Skills[0].Name = "changed"
	if catalog.skills[0].Name != "review" {
		t.Fatal("expected result to own its skill slice")
	}
	if service.Root() != "skills" {
		t.Fatalf("expected normalized root, got %q", service.Root())
	}
}

func TestCatalogServicePropagatesRefreshError(t *testing.T) {
	expected := errors.New("scan failed")
	_, err := (CatalogService{Catalog: &fakeSkillCatalog{err: expected}}).List(
		context.Background(),
		ListSkillsQuery{Refresh: true},
	)
	if !errors.Is(err, expected) {
		t.Fatalf("expected refresh error, got %v", err)
	}
}

type fakeSkillCatalog struct {
	root     string
	skills   []domainskill.Skill
	reloaded bool
	err      error
}

func (c *fakeSkillCatalog) Reload(context.Context) error {
	c.reloaded = true
	return c.err
}

func (c *fakeSkillCatalog) List() []domainskill.Skill {
	return c.skills
}

func (c *fakeSkillCatalog) Root() string {
	return c.root
}
