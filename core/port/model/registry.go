package model

type Registry interface {
	GetModel(name string) ChatModelPort
	HasModel(name string) bool
	ListModels() []ModelInfo
}

type MutableRegistry interface {
	Registry
	SetModelInfo(modelName string, model ChatModelPort, info ModelInfo)
}
