package command

import "myai/core/domain/generation"

type AddConfig struct {
	ID                        string
	Name                      string
	Provider                  string
	BaseURL                   string
	APIKey                    string
	ModelName                 string
	IsDefault                 bool
	DefaultGenerationSettings generation.Settings
}
