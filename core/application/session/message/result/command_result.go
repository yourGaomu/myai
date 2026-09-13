package result

import (
	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

type Command struct {
	Session            *session.Session
	Input              string
	RuntimeInstruction string
	RAGContext         string
	AppendedMessages   []domainmessage.Message
	Appended           bool
}
