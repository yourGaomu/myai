package command

type Ingest struct {
	KnowledgeBaseID string
	AssetID         string
}

type Retry struct {
	JobID string
}

type Delete struct {
	DocumentID string
	Reason     string
}
