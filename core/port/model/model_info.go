package model

import "myai/core/domain/generation"

type ModelInfo struct {
	ID                        string
	Name                      string
	Provider                  string
	ModelName                 string
	Enabled                   bool
	IsDefault                 bool
	DefaultGenerationSettings generation.Settings
}
