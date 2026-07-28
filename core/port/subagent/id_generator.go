package subagent

type IDGenerator interface {
	NewID() string
}
