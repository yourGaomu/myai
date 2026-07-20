package result

type ContextHit struct {
	KnowledgeBaseID string
	DocumentID      string
	ChunkID         string
	Text            string
	SourceName      string
	SourceLocation  string
	Rank            int
}
