package result

import domain "myai/core/domain/imagemodel"

type TestResult struct {
	OK        bool
	Message   string
	LatencyMS int64
	MIMEType  string
}
type MutationResult struct {
	Action         string
	Model          *domain.Summary
	Models         []domain.Summary
	DefaultModelID string
	Message        string
}
