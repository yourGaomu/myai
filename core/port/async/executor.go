package async

type Executor interface {
	Submit(task func()) error
}
