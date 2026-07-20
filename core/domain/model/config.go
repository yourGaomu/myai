package model

import (
	"myai/core/domain/generation"
	"time"
)

type Config struct {
	ID                        string
	Name                      string
	Provider                  string
	BaseURL                   string
	APIKey                    string
	ModelName                 string
	Enabled                   bool
	IsDefault                 bool
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	DefaultGenerationSettings generation.Settings
}
