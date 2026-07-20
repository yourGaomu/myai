package command

type Documents struct {
	KnowledgeBaseID string
	IncludeDeleted  bool
	JobLimit        int
}

type IndexProfiles struct {
	IncludeDeleted bool
}
