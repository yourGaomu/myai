package service

import (
	"context"
	"strings"

	skillapi "myai/core/application/skill/api"
	skillport "myai/core/application/skill/port"
	skillquery "myai/core/application/skill/query"
	skillresult "myai/core/application/skill/result"
)

type CatalogService struct {
	// 应用层只依赖 Skill Catalog；文件扫描和解析由 core/skill 的实现负责。
	Catalog skillport.Catalog
}

var _ skillapi.CatalogService = CatalogService{}

func (s CatalogService) List(ctx context.Context, query skillquery.ListSkills) (skillresult.ListSkills, error) {
	if s.Catalog == nil {
		return skillresult.ListSkills{}, nil
	}
	if query.Refresh {
		if err := s.Catalog.Reload(ctx); err != nil {
			return skillresult.ListSkills{}, err
		}
	}

	skills := s.Catalog.List()
	return skillresult.ListSkills{Skills: append(skills[:0:0], skills...)}, nil
}

func (s CatalogService) Root() string {
	if s.Catalog == nil {
		return ""
	}
	return strings.TrimSpace(s.Catalog.Root())
}
