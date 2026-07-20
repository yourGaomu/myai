package command

type Search struct {
	Text             string
	KnowledgeBaseIDs []string
	CategoryIDs      []string
	TopK             int
	Strict           bool
}
