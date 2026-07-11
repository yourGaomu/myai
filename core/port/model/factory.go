package model

type Factory interface {
	CreateModel(config CreationConfig) (ChatModelPort, error)
}
