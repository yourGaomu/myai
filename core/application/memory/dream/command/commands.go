package command

type Run struct {
	Trigger        string
	CandidateLimit int
	MemoryLimit    int
}

type Get struct {
	RunID string
}

type List struct {
	Limit int
}
