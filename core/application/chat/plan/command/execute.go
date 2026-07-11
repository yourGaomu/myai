package command

import modelport "myai/core/port/model"

type Execute struct {
	SessionID string
	Stream    modelport.ChatStreamHandler
}
