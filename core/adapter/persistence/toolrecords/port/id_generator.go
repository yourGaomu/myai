package port

type IDGenerator interface {
	NewID() string
}
