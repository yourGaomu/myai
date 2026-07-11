package skillapp

import (
	skillport "myai/core/application/skill/port"
	skillquery "myai/core/application/skill/query"
	skillresult "myai/core/application/skill/result"
	skillservice "myai/core/application/skill/service"
)

type Catalog = skillport.Catalog
type ListSkillsQuery = skillquery.ListSkills
type ListSkillsResult = skillresult.ListSkills
type CatalogService = skillservice.CatalogService
