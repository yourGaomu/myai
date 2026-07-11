package port

import modelport "myai/core/port/model"

type ModelProvider interface {
	GetModel(name string) modelport.ChatModelPort
}
