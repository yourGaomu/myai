package result

type Context struct {
	Triggered bool
	Query     string
	Prompt    string
	MemoryIDs []string
	Error     string
}
