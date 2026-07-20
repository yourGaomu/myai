package command

type List struct {
	IncludeDeleted bool
}

type CreateCategory struct {
	Name      string
	ParentID  string
	SortOrder int
}

type MoveCategory struct {
	CategoryID string
	ParentID   string
	SortOrder  int
}

type DeleteCategory struct {
	CategoryID string
	Reason     string
	Recursive  bool
}

type CreateKnowledgeBase struct {
	CategoryID           string
	Name                 string
	Description          string
	RAGEnabled           bool
	ActiveIndexProfileID string
}

type UpdateKnowledgeBase struct {
	KnowledgeBaseID      string
	CategoryID           string
	Name                 string
	Description          string
	RAGEnabled           bool
	ActiveIndexProfileID string
}

type DeleteKnowledgeBase struct {
	KnowledgeBaseID string
	Reason          string
}
