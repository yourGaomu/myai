package command

type Ingest struct {
	KnowledgeBaseID string
	URL             string
	Code            string
}

type Retry struct {
	JobID string
}

type Delete struct {
	DocumentID string
	Reason     string
}
